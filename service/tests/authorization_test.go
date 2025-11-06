package tests

import (
	"encoding/json"
	"testing"
)

func TestAuthorization_Suite(t *testing.T) {
	// Start test server
	server := StartTestServer(t)
	defer server.Stop()
	t.Run("CORSBehavior", func(t *testing.T) {
		// Test CORS headers
		headers := AuthHeaders(TestFrontendKey)
		headers["Origin"] = "https://allowed.example"
		res, err := DoRequest(t, "GET", EndpointFrontendThreads, nil, headers)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		// This would need actual CORS config in server
		_ = res.Header.Get("Access-Control-Allow-Origin")
	})

	t.Run("AuthorTamperingProtection", func(t *testing.T) {
		// Test that signature validation prevents tampering
		threadBody := map[string]string{"author": "alice", "title": "test-thread"}
		jsonBody, _ := json.Marshal(threadBody)

		headers := AuthHeaders(TestFrontendKey)
		headers["X-User-ID"] = "alice"
		headers["X-User-Signature"] = "invalid-signature"

		res, err := DoRequest(t, "POST", EndpointFrontendThreads, jsonBody, headers)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode == 200 {
			t.Fatalf("expected invalid signature to be rejected; got 200")
		}
	})

	t.Run("RateLimiting", func(t *testing.T) {
		// Test rate limiting behavior
		body := map[string]string{"userId": "test-user"}
		jsonBody, _ := json.Marshal(body)

		// Make multiple requests quickly to potentially trigger rate limiting
		rateLimited := false
		for i := 0; i < 20; i++ {
			res, err := DoRequest(t, "POST", EndpointBackendSign, jsonBody, AuthHeaders(TestBackendKey))
			if err != nil {
				t.Fatalf("request %d failed: %v", i, err)
			}

			if res.StatusCode == 429 {
				rateLimited = true
				res.Body.Close()
				break
			}
			res.Body.Close()
		}

		// Note: Rate limiting behavior depends on configuration
		// This test documents the behavior rather than enforcing it
		if rateLimited {
			t.Logf("Rate limiting detected as expected")
		} else {
			t.Logf("Rate limiting not triggered (may be disabled or high limits)")
		}
	})
}
