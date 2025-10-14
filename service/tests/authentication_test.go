package tests

// Objectives (from docs/tests.md):
// The single-file `authentication_test.go` contains all scenarios for authentication:
// 1. Start server with no API keys and verify anonymous and signature request handling.
// 2. Start server with frontend/backend/admin API keys and assert scope enforcement for each role.
// 3. Validate signing flows and misconfiguration behaviors.

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"

	utils "progressdb/tests/utils"
)

func TestAuthentication_Suite(t *testing.T) {
	// Subtest: Verify unsigned in-process request is rejected (no signature, no API key).
	t.Run("UnsignedCallRejected_InProcess", func(t *testing.T) {
		// start full server process for this test
		sp := utils.StartTestServerProcess(t)
		defer func() { _ = sp.Stop(t) }()

		// try to create a thread without auth; should be rejected
		req, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader([]byte(`{"title":"t1"}`)))
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
		req, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader([]byte(`{"title":"t1"}`)))
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
		status := utils.FrontendGetJSON(t, sp.Addr, "/admin/health", "", nil)
		if status == 200 {
			t.Fatalf("expected frontend key to be forbidden for admin endpoints; got 200")
		}

		// frontend key cannot call sign endpoint (requires backend role)
		sstatus := utils.FrontendPostJSON(t, sp.Addr, "/v1/_sign", map[string]string{"userId": "u"}, "", nil)
		if sstatus == 200 {
			t.Fatalf("expected frontend key to be forbidden for sign endpoint; got 200")
		}
	})

	// Subtest: E2E - backend API key can call signing and create messages but not admin endpoints.
	t.Run("E2E_BackendKey_Scopes", func(t *testing.T) {
		cfg := fmt.Sprintf(`server:
	  address: 127.0.0.1
	  port: {{PORT}}
	  db_path: {{WORKDIR}}/db
	security:
	  kms:
	    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
	  api_keys:
	    backend: ["%s"]
	    frontend: []
	    admin: []
	logging:
	  level: info
	`, utils.BackendAPIKey)
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// backend can call sign endpoint
		var sOut map[string]string
		sstatus := utils.BackendPostJSON(t, sp.Addr, "/v1/_sign", map[string]string{"userId": "bob"}, "", &sOut)
		if sstatus != 200 {
			t.Fatalf("expected backend key to be allowed for sign endpoint; status=%d", sstatus)
		}

		// backend can create messages by supplying author in body
		var tout map[string]interface{}
		mstatus := utils.BackendPostJSON(t, sp.Addr, "/v1/threads", map[string]string{"author": "bob", "title": "t-bob"}, "bob", &tout)
		if mstatus != 200 && mstatus != 201 && mstatus != 202 {
			t.Fatalf("expected backend to create message; status=%d", mstatus)
		}

		// backend cannot access admin endpoints
		astatus := utils.BackendGetJSON(t, sp.Addr, "/admin/health", "", nil)
		if astatus == 200 {
			t.Fatalf("expected backend key to be forbidden for admin endpoints; got 200")
		}
	})

	// Subtest: E2E - admin API key can access admin endpoints and create messages but cannot call sign endpoint.
	t.Run("E2E_AdminKey_Scopes", func(t *testing.T) {
		cfg := fmt.Sprintf(`server:
	  address: 127.0.0.1
	  port: {{PORT}}
	  db_path: {{WORKDIR}}/db
	security:
	  kms:
	    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
	  api_keys:
	    backend: ["%s"]
	    frontend: []
	    admin: ["admin-secret"]
	logging:
	  level: info
	`, utils.SigningSecret)
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// admin can access admin health
		astatus := utils.DoAdminJSON(t, sp.Addr, "GET", "/admin/health", nil, nil)
		if astatus != 200 {
			t.Fatalf("expected admin health 200; got %d", astatus)
		}

		// admin cannot call sign endpoint (requires backend role)
		sstatus := utils.BackendPostJSON(t, sp.Addr, "/v1/_sign", map[string]string{"userId": "u"}, "", nil)
		if sstatus == 200 {
			t.Fatalf("expected admin key to be forbidden for sign endpoint; got 200")
		}

		// admin should NOT be able to call non-/admin endpoints (enforce admin-only routes)
		mstatus := utils.BackendPostJSON(t, sp.Addr, "/v1/threads", map[string]string{"author": "admin", "title": "t-admin"}, "admin", nil)
		if mstatus == 200 {
			t.Fatalf("expected admin key to be forbidden for non-admin endpoints; got 200")
		}
	})
}
