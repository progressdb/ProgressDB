package tests

// Objectives (from docs/tests.md):
// The single-file `authentication_test.go` contains all scenarios for authentication:
// 1. Start server with no API keys and verify anonymous and signature request handling.
// 2. Start server with frontend/backend/admin API keys and assert scope enforcement for each role.
// 3. Validate signing flows and misconfiguration behaviors.

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	utils "progressdb/tests/utils"
)

func TestAuthentication_Suite(t *testing.T) {
	// Subtest: Verify unsigned in-process request is rejected (no signature, no API key).
	t.Run("UnsignedCallRejected_InProcess", func(t *testing.T) {
		// existing lightweight in-process check retained for fast feedback
		srv := utils.SetupServer(t)
		defer srv.Close()

		req, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader([]byte(`{"body":{}}`)))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode == 200 {
			t.Fatalf("expected error for unsigned request; got 200")
		}
	})

	// Subtest: E2E - start server with no API keys; check health and that unauthenticated POSTs are rejected.
	t.Run("E2E_NoKeys_StartAndBehavior", func(t *testing.T) {
		// config with no api keys
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
logging:
  level: info
`
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// health should be available
		res, err := http.Get(sp.Addr + "/healthz")
		if err != nil {
			t.Fatalf("healthz request failed: %v", err)
		}
		if res.StatusCode != 200 {
			t.Fatalf("expected healthz 200 got %d", res.StatusCode)
		}

		// unauthenticated POST to messages should be rejected (no signature, no api key)
		req, _ := http.NewRequest("POST", sp.Addr+"/v1/messages", bytes.NewReader([]byte(`{"body":{}}`)))
		cre, err := http.DefaultClient.Do(req)
		t.Logf("POST /v1/messages response: %+v", cre)
		if err != nil {
			t.Fatalf("POST /v1/messages failed: %v", err)
		}
		defer cre.Body.Close()
		body, _ := io.ReadAll(cre.Body)
		t.Logf("response status=%d body=%s", cre.StatusCode, string(body))
		if cre.StatusCode == 200 {
			t.Fatalf("expected unauthenticated POST /v1/messages to be rejected; got 200")
		}
	})

	// Subtest: E2E - frontend API key should be limited in scope (no admin, no sign endpoint).
	t.Run("E2E_FrontendKey_Scopes", func(t *testing.T) {
		cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    backend: []
    frontend: ["frontend-secret"]
    admin: []
logging:
  level: info
`
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// frontend key cannot access admin endpoints
		req, _ := http.NewRequest("GET", sp.Addr+"/admin/health", nil)
		req.Header.Set("Authorization", "Bearer frontend-secret")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("admin health request failed: %v", err)
		}
		if res.StatusCode == 200 {
			t.Fatalf("expected frontend key to be forbidden for admin endpoints; got 200")
		}

		// frontend key cannot call sign endpoint (requires backend role)
		sreq, _ := http.NewRequest("POST", sp.Addr+"/v1/_sign", bytes.NewReader([]byte(`{"userId":"u"}`)))
		sreq.Header.Set("Authorization", "Bearer frontend-secret")
		sres, err := http.DefaultClient.Do(sreq)
		if err != nil {
			t.Fatalf("sign request failed: %v", err)
		}
		if sres.StatusCode == 200 {
			t.Fatalf("expected frontend key to be forbidden for sign endpoint; got 200")
		}
	})

	// Subtest: E2E - backend API key can call signing and create messages but not admin endpoints.
	t.Run("E2E_BackendKey_Scopes", func(t *testing.T) {
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
logging:
  level: info
`
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// backend can call sign endpoint
		sreq, _ := http.NewRequest("POST", sp.Addr+"/v1/_sign", bytes.NewReader([]byte(`{"userId":"bob"}`)))
		sreq.Header.Set("Authorization", "Bearer backend-secret")
		sres, err := http.DefaultClient.Do(sreq)
		if err != nil {
			t.Fatalf("sign request failed: %v", err)
		}
		if sres.StatusCode != 200 {
			t.Fatalf("expected backend key to be allowed for sign endpoint; status=%d", sres.StatusCode)
		}

		// backend can create messages by supplying author in body
		mreq, _ := http.NewRequest("POST", sp.Addr+"/v1/messages", bytes.NewReader([]byte(`{"author":"bob","body":{}}`)))
		mreq.Header.Set("Authorization", "Bearer backend-secret")
		mres, err := http.DefaultClient.Do(mreq)
		if err != nil {
			t.Fatalf("create message failed: %v", err)
		}
		if mres.StatusCode != 200 && mres.StatusCode != 201 {
			t.Fatalf("expected backend to create message; status=%d", mres.StatusCode)
		}

		// backend cannot access admin endpoints
		areq, _ := http.NewRequest("GET", sp.Addr+"/admin/health", nil)
		areq.Header.Set("Authorization", "Bearer backend-secret")
		ares, err := http.DefaultClient.Do(areq)
		if err != nil {
			t.Fatalf("admin health request failed: %v", err)
		}
		if ares.StatusCode == 200 {
			t.Fatalf("expected backend key to be forbidden for admin endpoints; got 200")
		}
	})

	// Subtest: E2E - admin API key can access admin endpoints and create messages but cannot call sign endpoint.
	t.Run("E2E_AdminKey_Scopes", func(t *testing.T) {
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
    admin: ["admin-secret"]
logging:
  level: info
`
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// admin can access admin health
		areq, _ := http.NewRequest("GET", sp.Addr+"/admin/health", nil)
		areq.Header.Set("Authorization", "Bearer admin-secret")
		ares, err := http.DefaultClient.Do(areq)
		if err != nil {
			t.Fatalf("admin health failed: %v", err)
		}
		if ares.StatusCode != 200 {
			t.Fatalf("expected admin health 200; got %d", ares.StatusCode)
		}

		// admin cannot call sign endpoint (requires backend role)
		sreq, _ := http.NewRequest("POST", sp.Addr+"/v1/_sign", bytes.NewReader([]byte(`{"userId":"u"}`)))
		sreq.Header.Set("Authorization", "Bearer admin-secret")
		sres, err := http.DefaultClient.Do(sreq)
		if err != nil {
			t.Fatalf("sign request failed: %v", err)
		}
		if sres.StatusCode == 200 {
			t.Fatalf("expected admin key to be forbidden for sign endpoint; got 200")
		}

		// admin can create messages by providing author
		mreq, _ := http.NewRequest("POST", sp.Addr+"/v1/messages", bytes.NewReader([]byte(`{"author":"admin","body":{}}`)))
		mreq.Header.Set("Authorization", "Bearer admin-secret")
		mres, err := http.DefaultClient.Do(mreq)
		if err != nil {
			t.Fatalf("create message failed: %v", err)
		}
		if mres.StatusCode != 200 && mres.StatusCode != 201 {
			t.Fatalf("expected admin to create message; status=%d", mres.StatusCode)
		}
	})
}
