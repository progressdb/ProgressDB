package tests

import (
	"encoding/json"
	"testing"
)

func TestHandlers_E2E_ThreadsMessagesCRUD(t *testing.T) {
	// Start test server
	server := StartTestServer(t)
	defer server.Stop()
	t.Run("ThreadCreation", func(t *testing.T) {
		// Test basic thread creation
		threadBody := map[string]string{"author": "alice", "title": "test-thread"}
		jsonBody, _ := json.Marshal(threadBody)

		// Generate signed headers for frontend request
		signedHeaders, err := SignedAuthHeaders(TestFrontendKey, "alice")
		if err != nil {
			t.Fatalf("failed to generate signed headers: %v", err)
		}

		res, err := DoRequest(t, "POST", EndpointFrontendThreads, jsonBody, signedHeaders)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != 200 && res.StatusCode != 201 && res.StatusCode != 202 {
			t.Fatalf("expected thread creation to succeed; got %d", res.StatusCode)
		}

		var response map[string]interface{}
		json.NewDecoder(res.Body).Decode(&response)

		if response == nil {
			t.Fatalf("response is nil")
		}

		if _, ok := response["key"]; !ok {
			t.Fatalf("expected thread key in response")
		}
	})

	t.Run("MessageCreation", func(t *testing.T) {
		// First create a thread
		threadBody := map[string]string{"author": "alice", "title": "message-thread"}
		threadJson, _ := json.Marshal(threadBody)

		// Generate signed headers for frontend request
		signedHeaders, err := SignedAuthHeaders(TestFrontendKey, "alice")
		if err != nil {
			t.Fatalf("failed to generate signed headers: %v", err)
		}

		res, err := DoRequest(t, "POST", EndpointFrontendThreads, threadJson, signedHeaders)
		if err != nil {
			t.Fatalf("thread creation failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != 200 && res.StatusCode != 201 && res.StatusCode != 202 {
			t.Fatalf("expected thread creation to succeed; got %d", res.StatusCode)
		}

		var threadResponse map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&threadResponse); err != nil {
			t.Fatalf("failed to decode thread response: %v", err)
		}

		if threadResponse == nil {
			t.Fatalf("thread response is nil")
		}

		threadID, ok := threadResponse["key"].(string)
		if !ok {
			t.Fatalf("expected thread key in response, got %v", threadResponse["key"])
		}

		// Now create a message
		messageBody := map[string]interface{}{
			"author": "alice",
			"body":   map[string]string{"text": "hello world"},
			"thread": threadID,
		}
		messageJson, _ := json.Marshal(messageBody)

		msgRes, err := DoRequest(t, "POST", ThreadMessagesURL(threadID), messageJson, signedHeaders)
		if err != nil {
			t.Fatalf("message creation failed: %v", err)
		}
		defer msgRes.Body.Close()

		if msgRes.StatusCode != 200 && msgRes.StatusCode != 201 && msgRes.StatusCode != 202 {
			t.Fatalf("expected message creation to succeed; got %d", msgRes.StatusCode)
		}
	})

	t.Run("MessageListing", func(t *testing.T) {
		// Test message listing with pagination
		signedHeaders, err := SignedAuthHeaders(TestFrontendKey, "alice")
		if err != nil {
			t.Fatalf("failed to generate signed headers: %v", err)
		}

		res, err := DoRequest(t, "GET", ThreadMessagesURL("test-thread")+"?limit=1", nil, signedHeaders)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode == 200 {
			var response struct {
				Messages []map[string]interface{} `json:"messages"`
			}
			json.NewDecoder(res.Body).Decode(&response)

			if len(response.Messages) > 1 {
				t.Fatalf("expected max 1 message due to limit; got %d", len(response.Messages))
			}
		}
	})
}
