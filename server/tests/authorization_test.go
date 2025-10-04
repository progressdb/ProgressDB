package tests

// Objectives (from docs/tests.md):
// 1. Validate CORS configuration affects allowed origins as configured.
// 2. Validate rate limiting per-config (RPS/Burst) and ensure limits are enforced.
// 3. Validate role-based resource visibility (soft-deleted threads visible to admins only).
// 4. Validate author/ownership checks and header/signature tampering protections.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"progressdb/pkg/models"
	"progressdb/pkg/store"
	utils "progressdb/tests/utils"
)

func TestAuthorization_Suite(t *testing.T) {
	// Subtest: Ensure admins can access soft-deleted threads while non-admins (even signed users) cannot.
	t.Run("DeletedThreadAdminAccess", func(t *testing.T) {
		srv := utils.SetupServer(t)
		defer srv.Close()

		th := models.Thread{
			ID:        "auth-thread-1",
			Title:     "t",
			Author:    "alice",
			Deleted:   true,
			DeletedTS: time.Now().Add(-24 * time.Hour).UnixNano(),
		}
		b, _ := json.Marshal(th)
		if err := store.SaveThread(th.ID, string(b)); err != nil {
			t.Fatalf("SaveThread: %v", err)
		}

		q := url.Values{}
		q.Set("author", "alice")
		req, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+th.ID+"?"+q.Encode(), nil)
		req.Header.Set("X-Role-Name", "admin")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("expected admin to access deleted thread; status=%d", res.StatusCode)
		}

		sig := utils.SignHMAC("signsecret", "alice")
		sreq, _ := http.NewRequest("GET", srv.URL+"/v1/threads/"+th.ID+"", nil)
		sreq.Header.Set("X-User-ID", "alice")
		sreq.Header.Set("X-User-Signature", sig)
		sres, err := http.DefaultClient.Do(sreq)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer sres.Body.Close()
		if sres.StatusCode == 200 {
			t.Fatalf("expected signed non-admin to not see deleted thread; got 200")
		}
	})

	// Subtest: Verify CORS response headers only allow configured origins.
	t.Run("CORSBehavior", func(t *testing.T) {
		cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  cors:
    allowed_origins: ["https://allowed.example"]
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    admin: ["admin-secret"]
logging:
  level: info
`
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// request with allowed origin should include Access-Control-Allow-Origin
		req, _ := http.NewRequest("GET", sp.Addr+"/v1/threads", nil)
		req.Header.Set("Origin", "https://allowed.example")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != "https://allowed.example" {
			t.Fatalf("expected Access-Control-Allow-Origin header for allowed origin; got %q", got)
		}

		// request with disallowed origin should not set header
		req2, _ := http.NewRequest("GET", sp.Addr+"/v1/threads", nil)
		req2.Header.Set("Origin", "https://evil.example")
		res2, err := http.DefaultClient.Do(req2)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if got := res2.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("expected no Access-Control-Allow-Origin header for disallowed origin; got %q", got)
		}
	})

	// Subtest: Start server with strict rate limit and validate throttling behavior on quick successive requests.
	t.Run("RateLimitBehavior", func(t *testing.T) {
		// start server with strict rate limit: 1 rps, burst 1
		cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    backend: []
    frontend: []
    admin: []
  rate_limit:
    rps: 1
    burst: 1
logging:
  level: info
`
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// perform multiple concurrent requests to a permissive endpoint (/healthz) and expect at least one to be rate limited (429)
		const tries = 100
		const concurrency = 10
		got429 := int32(0)
		var wg sync.WaitGroup

		client := &http.Client{}
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for j := 0; j < tries/concurrency; j++ {
					res, err := client.Get(sp.Addr + "/healthz")
					if err != nil {
						t.Errorf("healthz request failed: %v", err)
						continue
					}
					// Log the response for each request
					bodyBytes, _ := io.ReadAll(res.Body)
					t.Logf("[worker %d, req %d] status: %d, body: %q", worker, j, res.StatusCode, string(bodyBytes))
					if res.StatusCode == 429 {
						atomic.AddInt32(&got429, 1)
					}
					_ = res.Body.Close()
					// small backoff to increase chance of hitting rate limit window
					time.Sleep(10 * time.Millisecond)
				}
			}(i)
		}
		wg.Wait()
		if got429 == 0 {
			t.Fatalf("expected at least one 429 rate-limited response across %d quick concurrent requests", tries)
		}
	})

	// Subtest: Verify signature-based author protection prevents tampering with X-User-ID and author fields.
	t.Run("AuthorTamperingProtection", func(t *testing.T) {
		// start server with backend key for signing
		cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    backend: ["backend-secret"]
    admin: []
logging:
  level: info
`
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// create thread as alice using signature headers
		sig := utils.SignHMAC("backend-secret", "alice")
		thBody := []byte(`{"author":"alice","title":"t1"}`)
		creq, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(thBody))
		creq.Header.Set("X-User-ID", "alice")
		creq.Header.Set("X-User-Signature", sig)
		cres, err := http.DefaultClient.Do(creq)
		if err != nil {
			t.Fatalf("create thread failed: %v", err)
		}
		if cres.StatusCode != 200 && cres.StatusCode != 201 {
			t.Fatalf("unexpected create thread status: %d", cres.StatusCode)
		}
		defer cres.Body.Close()
		var tout map[string]interface{}
		if err := json.NewDecoder(cres.Body).Decode(&tout); err != nil {
			t.Fatalf("failed to decode create thread response: %v", err)
		}
		tid := tout["id"].(string)

		// attempt update with mismatched header (bob) but signature for alice -> expect 403
		upBody := []byte(`{"title":"updated"}`)
		ureq, _ := http.NewRequest("PUT", sp.Addr+"/v1/threads/"+tid, bytes.NewReader(upBody))
		ureq.Header.Set("X-User-ID", "bob")
		ureq.Header.Set("X-User-Signature", sig)
		ures, err := http.DefaultClient.Do(ureq)
		if err != nil {
			t.Fatalf("update request failed: %v", err)
		}
		if ures.StatusCode == 200 {
			t.Fatalf("expected update with mismatched signature/header to be forbidden; got 200")
		}

		// attempt update with body author mismatch
		upBody2 := []byte(`{"author":"mallory","title":"updated2"}`)
		ureq2, _ := http.NewRequest("PUT", sp.Addr+"/v1/threads/"+tid, bytes.NewReader(upBody2))
		ureq2.Header.Set("X-User-ID", "alice")
		ureq2.Header.Set("X-User-Signature", sig)
		ures2, err := http.DefaultClient.Do(ureq2)
		if err != nil {
			t.Fatalf("update2 request failed: %v", err)
		}
		if ures2.StatusCode == 200 {
			t.Fatalf("expected update with mismatched body author to be forbidden; got 200")
		}
	})
}
