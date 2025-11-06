package tests

import (
	"encoding/json"
	"testing"
)

func TestAuthentication_Suite(t *testing.T) {
	// Start test server
	server := StartTestServer(t)
	defer server.Stop()
	t.Run("UnsignedCallRejected", func(t *testing.T) {
		// Test that unauthenticated requests are rejected
		res, err := DoRequest(t, "POST", EndpointFrontendThreads, []byte(`{"title":"t1"}`), nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode == 200 {
			t.Fatalf("expected unauthenticated request to be rejected; got 200")
		}
	})

	t.Run("FrontendKeyScope", func(t *testing.T) {
		// Test that frontend key can't access admin endpoints
		res, err := DoRequest(t, "GET", EndpointAdminHealth, nil, AuthHeaders(TestFrontendKey))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode == 200 {
			t.Fatalf("expected frontend key to be forbidden for admin endpoints; got 200")
		}
	})

	t.Run("BackendKeyScope", func(t *testing.T) {
		// Test that backend key can call sign endpoint
		body := map[string]string{"userId": "test-user"}
		jsonBody, _ := json.Marshal(body)

		t.Logf("Making request to %s with backend key %s", EndpointBackendSign, TestBackendKey)
		res, err := DoRequest(t, "POST", EndpointBackendSign, jsonBody, AuthHeaders(TestBackendKey))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		t.Logf("Got response status: %d", res.StatusCode)
		if res.StatusCode != 200 {
			t.Fatalf("expected backend key to access sign endpoint; got %d", res.StatusCode)
		}
	})

	t.Run("AdminKeyScope", func(t *testing.T) {
		// Test that admin key can access admin endpoints
		t.Logf("Making request to %s with admin key %s", EndpointAdminHealth, TestAdminKey)
		res, err := DoRequest(t, "GET", EndpointAdminHealth, nil, AuthHeaders(TestAdminKey))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		t.Logf("Got response status: %d", res.StatusCode)
		if res.StatusCode != 200 {
			t.Fatalf("expected admin key to access admin endpoints; got %d", res.StatusCode)
		}

		// Test that admin key can't call sign endpoint
		res2, err := DoRequest(t, "POST", EndpointBackendSign, []byte(`{"userId":"test"}`), AuthHeaders(TestAdminKey))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res2.Body.Close()

		if res2.StatusCode == 200 {
			t.Fatalf("expected admin key to be forbidden for sign endpoint; got 200")
		}
	})
}
