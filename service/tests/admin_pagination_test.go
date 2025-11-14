package tests

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
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
		threadBody["title"] = fmt.Sprintf("test-thread-%d", i)
		jsonBody, _ = json.Marshal(threadBody)
		res, err := DoRequest(t, "POST", EndpointFrontendThreads, jsonBody, signedHeaders)
		if err != nil {
			t.Fatalf("failed to create thread %d: %v", i, err)
		}
		if res.StatusCode != 200 && res.StatusCode != 202 {
			t.Fatalf("thread %d creation failed with status %d", i, res.StatusCode)
		}
		res.Body.Close()
	}

	// Wait for async processing to complete - poll until we have keys
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res, err := DoRequest(t, "GET", EndpointAdminKeys+"?limit=1", nil, AuthHeaders(TestAdminKey))
		if err != nil {
			t.Fatalf("health check request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode == 200 {
			var response map[string]interface{}
			if err := json.NewDecoder(res.Body).Decode(&response); err == nil {
				if keys, ok := response["keys"].([]interface{}); ok && len(keys) > 0 {
					t.Logf("Async processing completed - found %d keys", len(keys))
					break
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
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

		// Check pagination metadata is nested under "pagination"
		pagination, ok := response["pagination"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected pagination object in response")
		}

		if _, ok := pagination["has_before"]; !ok {
			t.Fatalf("expected has_before in pagination response")
		}
		if _, ok := pagination["has_after"]; !ok {
			t.Fatalf("expected has_after in pagination response")
		}
		if _, ok := pagination["before_anchor"]; !ok {
			t.Fatalf("expected before_anchor in pagination response")
		}
		if _, ok := pagination["after_anchor"]; !ok {
			t.Fatalf("expected after_anchor in pagination response")
		}

		// Check that we got keys
		keys, ok := response["keys"].([]interface{})
		if !ok {
			t.Fatalf("expected keys array in response")
		}
		if len(keys) != 5 {
			t.Fatalf("expected 5 keys; got %d", len(keys))
		}

		t.Logf("Initial load: got %d keys, after_anchor=%v, before_anchor=%v", len(keys), pagination["after_anchor"], pagination["before_anchor"])
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

		pagination, ok := initialResponse["pagination"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected pagination object in initial response")
		}
		beforeAnchor := pagination["before_anchor"].(string)

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

		pagination, ok := initialResponse["pagination"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected pagination object in initial response")
		}
		afterAnchor := pagination["after_anchor"].(string)

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
		decodeErr := json.NewDecoder(res.Body).Decode(&response)
		if decodeErr != nil {
			t.Fatalf("failed to decode JSON response: %v", decodeErr)
		}

		t.Logf("After pagination response: %+v", response)

		keys, ok := response["keys"]
		if !ok {
			t.Fatalf("expected keys field in response")
		}

		// Check if keys is null (which is valid for empty result)
		if keys == nil {
			t.Logf("Keys is null - no more items after this anchor")
		} else {
			keysArray, ok := keys.([]interface{})
			if !ok {
				t.Fatalf("expected keys to be array or null, got %T", keys)
			}
			t.Logf("After pagination got %d keys", len(keysArray))
		}
	})
}
