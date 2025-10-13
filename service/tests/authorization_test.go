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

	utils "progressdb/tests/utils"
)

func TestAuthorization_Suite(t *testing.T) {
	// ensure admins can access soft-deleted threads while non-admins (even signed users) cannot.
	t.Run("DeletedThreadAdminAccess", func(t *testing.T) {

		// create a thread then delete it (soft-delete) so admins can still access
		sp := utils.StartTestServerProcess(t)
		defer func() { _ = sp.Stop(t) }()

		user := "alice"
		tid, _ := utils.CreateThreadAPI(t, sp.Addr, user, "t")

		// soft-delete as the author
		sig := utils.SignHMAC(utils.SigningSecret, user)
		dreq, _ := http.NewRequest("DELETE", sp.Addr+"/v1/threads/"+tid, nil)
		dreq.Header.Set("X-User-ID", user)
		dreq.Header.Set("X-User-Signature", sig)
		dreq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
		dres, err := http.DefaultClient.Do(dreq)
		if err != nil {
			t.Fatalf("delete request failed: %v", err)
		}
		if dres.StatusCode != 202 {
			t.Fatalf("expected delete to return 202; got %d", dres.StatusCode)
		}

		q := url.Values{}
		q.Set("author", "alice")
		req, _ := http.NewRequest("GET", sp.Addr+"/admin/threads", nil)
		req.Header.Set("Authorization", "Bearer "+utils.AdminAPIKey)
		// admin API key is sufficient for /admin routes
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("expected admin to access deleted thread; status=%d", res.StatusCode)
		}

		utils.WaitForThreadVisible(t, sp.Addr, tid, "alice", 5*time.Second)
		sreq, _ := http.NewRequest("GET", sp.Addr+"/v1/threads/"+tid+"", nil)
		sreq.Header.Set("X-User-ID", "alice")
		sreq.Header.Set("X-User-Signature", utils.SignHMAC(utils.SigningSecret, "alice"))
		sreq.Header.Set("Authorization", "Bearer "+utils.FrontendAPIKey)
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
    backend: ["backend-secret"]
    frontend: ["frontend-secret"]
    admin: ["admin-secret"]
logging:
  level: info
`
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// request with allowed origin should include Access-Control-Allow-Origin
		req, _ := http.NewRequest("GET", sp.Addr+"/v1/threads", nil)
		req.Header.Set("Origin", "https://allowed.example")
		req.Header.Set("Authorization", "Bearer frontend-secret")
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
		req2.Header.Set("Authorization", "Bearer frontend-secret")
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
    backend: ["backend-secret"]
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
					// call POST /v1/_sign which accepts backend API keys
					reqBody := []byte(`{"userId":"u"}`)
					req, _ := http.NewRequest("POST", sp.Addr+"/v1/_sign", bytes.NewReader(reqBody))
					req.Header.Set("Authorization", "Bearer backend-secret")
					req.Header.Set("Content-Type", "application/json")
					res, err := client.Do(req)
					if err != nil {
						t.Errorf("sign request failed: %v", err)
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
    frontend: ["frontend-secret"]
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
		creq.Header.Set("Authorization", "Bearer frontend-secret")
		cres, err := http.DefaultClient.Do(creq)
		if err != nil {
			t.Fatalf("create thread failed: %v", err)
		}
		if cres.StatusCode != 200 && cres.StatusCode != 201 && cres.StatusCode != 202 {
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
		ureq.Header.Set("Authorization", "Bearer frontend-secret")
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
		ureq2.Header.Set("Authorization", "Bearer frontend-secret")
		ures2, err := http.DefaultClient.Do(ureq2)
		if err != nil {
			t.Fatalf("update2 request failed: %v", err)
		}
		if ures2.StatusCode == 200 {
			t.Fatalf("expected update with mismatched body author to be forbidden; got 200")
		}
	})
}
