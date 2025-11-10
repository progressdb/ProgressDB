package tests

import (
	"encoding/json"
	"testing"
)

func TestAdminKeysPagination(t *testing.T) {
	server := StartTestServer(t)
	defer server.Stop()

	// Create some test data first
	threadBody := map[string]string{"author": "alice", "title": "test-thread-1"}
	jsonBody, _ := json.Marshal(threadBody)
	signedHeaders, _ := SignedAuthHeaders(TestFrontendKey, "alice")

	// Create multiple threads for pagination testing
	for i := 1; i <= 15; i++ {
		threadBody["title"] = "test-thread-" + string(rune(i))
		jsonBody, _ = json.Marshal(threadBody)
		res, err := DoRequest(t, "POST", EndpointFrontendThreads, jsonBody, signedHeaders)
		if err != nil {
			t.Fatalf("failed to create thread %d: %v", i, err)
		}
		res.Body.Close()
	}

	t.Run("InitialLoad", func(t *testing.T) {
		// Test initial load without pagination
		res, err := DoRequest(t, "GET", EndpointAdminKeys+"?limit=5", nil, AuthHeaders(TestAdminKey))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			t.Fatalf("expected 200; got %d", res.StatusCode)
		}

		var response map[string]interface{}
		json.NewDecoder(res.Body).Decode(&response)

		if response == nil {
			t.Fatalf("response is nil")
		}

		// Check pagination metadata
		if _, ok := response["has_before"]; !ok {
			t.Fatalf("expected has_before in response")
		}
		if _, ok := response["has_after"]; !ok {
			t.Fatalf("expected has_after in response")
		}
		if _, ok := response["before_anchor"]; !ok {
			t.Fatalf("expected before_anchor in response")
		}
		if _, ok := response["after_anchor"]; !ok {
			t.Fatalf("expected after_anchor in response")
		}

		// Check that we got keys
		keys, ok := response["keys"].([]interface{})
		if !ok {
			t.Fatalf("expected keys array in response")
		}
		if len(keys) != 5 {
			t.Fatalf("expected 5 keys; got %d", len(keys))
		}

		// Log response for debugging
		t.Logf("Initial load response: %+v", response)
	})

	t.Run("BeforePagination", func(t *testing.T) {
		// First get initial load to get before_anchor
		res, err := DoRequest(t, "GET", EndpointAdminKeys+"?limit=5", nil, AuthHeaders(TestAdminKey))
		if err != nil {
			t.Fatalf("initial request failed: %v", err)
		}
		defer res.Body.Close()

		var initialResponse map[string]interface{}
		json.NewDecoder(res.Body).Decode(&initialResponse)

		beforeAnchor := initialResponse["before_anchor"].(string)

		// Now test pagination before that anchor
		res, err = DoRequest(t, "GET", EndpointAdminKeys+"?limit=3&before="+beforeAnchor, nil, AuthHeaders(TestAdminKey))
		if err != nil {
			t.Fatalf("pagination request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			t.Fatalf("expected 200; got %d", res.StatusCode)
		}

		var response map[string]interface{}
		json.NewDecoder(res.Body).Decode(&response)

		keys, ok := response["keys"].([]interface{})
		if !ok {
			t.Fatalf("expected keys array in response")
		}
		if len(keys) != 3 {
			t.Fatalf("expected 3 keys; got %d", len(keys))
		}

		t.Logf("Before pagination response: %+v", response)
	})

	t.Run("AfterPagination", func(t *testing.T) {
		// First get initial load to get after_anchor
		res, err := DoRequest(t, "GET", EndpointAdminKeys+"?limit=5", nil, AuthHeaders(TestAdminKey))
		if err != nil {
			t.Fatalf("initial request failed: %v", err)
		}
		defer res.Body.Close()

		var initialResponse map[string]interface{}
		json.NewDecoder(res.Body).Decode(&initialResponse)

		afterAnchor := initialResponse["after_anchor"].(string)

		// Now test pagination after that anchor
		res, err = DoRequest(t, "GET", EndpointAdminKeys+"?limit=3&after="+afterAnchor, nil, AuthHeaders(TestAdminKey))
		if err != nil {
			t.Fatalf("pagination request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			t.Fatalf("expected 200; got %d", res.StatusCode)
		}

		var response map[string]interface{}
		json.NewDecoder(res.Body).Decode(&response)

		keys, ok := response["keys"].([]interface{})
		if !ok {
			t.Fatalf("expected keys array in response")
		}

		t.Logf("After pagination response: %+v", response)
		_ = keys // Use the variable to avoid unused variable error
	})
}
