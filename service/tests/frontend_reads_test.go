package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"progressdb/pkg/models"
	"progressdb/pkg/store/pagination"
)

// TestReadThreadsList tests the ReadThreadsList endpoint comprehensively
func TestReadThreadsList(t *testing.T) {
	WithTestServer(t, func() {
		// Create test users
		user1 := "user_frontend_test_1"
		user2 := "user_frontend_test_2"

		// Get signed auth headers for user1
		authHeaders, err := SignedAuthHeaders(TestFrontendKey, user1)
		if err != nil {
			t.Fatalf("Failed to get signed auth headers: %v", err)
		}

		// Create test threads for user1
		threadKeys := createTestThreads(t, authHeaders, user1, 5)

		// Create some threads for user2 to test isolation
		user2Headers, err := SignedAuthHeaders(TestFrontendKey, user2)
		if err != nil {
			t.Fatalf("Failed to get signed auth headers for user2: %v", err)
		}
		createTestThreads(t, user2Headers, user2, 3)

		t.Run("Initial Load - Default Parameters", func(t *testing.T) {
			resp, err := DoRequest(t, "GET", EndpointFrontendThreads, nil, authHeaders)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			var response ThreadsListResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Should return user1's threads only
			if len(response.Threads) != 5 {
				t.Errorf("Expected 5 threads, got %d", len(response.Threads))
			}

			// Check pagination metadata
			if response.Pagination == nil {
				t.Fatal("Pagination metadata is nil")
			}

			if response.Pagination.Count != 5 {
				t.Errorf("Expected count 5, got %d", response.Pagination.Count)
			}

			if response.Pagination.Total != 5 {
				t.Errorf("Expected total 5, got %d", response.Pagination.Total)
			}

			// Initial load should have HasBefore=false, HasAfter=false (no more threads)
			if response.Pagination.HasBefore {
				t.Error("Initial load should not have HasBefore=true")
			}

			if response.Pagination.HasAfter {
				t.Error("Initial load should not have HasAfter=true when all threads fit")
			}

			// Verify thread ownership
			for _, thread := range response.Threads {
				if thread.Author != user1 {
					t.Errorf("Thread author mismatch: expected %s, got %s", user1, thread.Author)
				}
			}
		})

		t.Run("Pagination - Limit Parameter", func(t *testing.T) {
			// Test with limit=2
			url := EndpointFrontendThreads + "?limit=2"
			resp, err := DoRequest(t, "GET", url, nil, authHeaders)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			var response ThreadsListResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if len(response.Threads) != 2 {
				t.Errorf("Expected 2 threads, got %d", len(response.Threads))
			}

			if response.Pagination.Count != 2 {
				t.Errorf("Expected count 2, got %d", response.Pagination.Count)
			}

			// Should have HasAfter=true since there are more threads
			if !response.Pagination.HasAfter {
				t.Error("Should have HasAfter=true when there are more threads")
			}

			if response.Pagination.HasBefore {
				t.Error("Should have HasBefore=false for initial load")
			}
		})

		t.Run("Pagination - Before Query", func(t *testing.T) {
			// Use the second thread as reference point
			if len(threadKeys) < 2 {
				t.Fatal("Need at least 2 threads for before query test")
			}

			url := fmt.Sprintf("%s?before=%s&limit=2", EndpointFrontendThreads, threadKeys[1])
			resp, err := DoRequest(t, "GET", url, nil, authHeaders)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			var response ThreadsListResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Should return threads newer than the reference
			if len(response.Threads) == 0 {
				t.Error("Expected some threads in before query")
			}

			// Before query should have HasBefore=true if there are newer threads
			if !response.Pagination.HasBefore {
				t.Error("Before query should have HasBefore=true when there are newer threads")
			}

			// Check that BeforeAnchor is set correctly
			if response.Pagination.BeforeAnchor == "" {
				t.Error("BeforeAnchor should be set for before query")
			}
		})

		t.Run("Pagination - After Query", func(t *testing.T) {
			// Use the second thread as reference point
			if len(threadKeys) < 2 {
				t.Fatal("Need at least 2 threads for after query test")
			}

			url := fmt.Sprintf("%s?after=%s&limit=2", EndpointFrontendThreads, threadKeys[1])
			resp, err := DoRequest(t, "GET", url, nil, authHeaders)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			var response ThreadsListResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Should return threads older than the reference
			if len(response.Threads) == 0 {
				t.Error("Expected some threads in after query")
			}

			// After query should have HasBefore=true (newer threads exist)
			if !response.Pagination.HasBefore {
				t.Error("After query should have HasBefore=true")
			}

			// Check that AfterAnchor is set correctly
			if response.Pagination.AfterAnchor == "" {
				t.Error("AfterAnchor should be set for after query")
			}
		})

		t.Run("Pagination - Anchor Query", func(t *testing.T) {
			if len(threadKeys) < 3 {
				t.Fatal("Need at least 3 threads for anchor query test")
			}

			// Use the middle thread as anchor
			anchorThread := threadKeys[2]
			url := fmt.Sprintf("%s?anchor=%s&limit=2", EndpointFrontendThreads, anchorThread)
			resp, err := DoRequest(t, "GET", url, nil, authHeaders)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			var response ThreadsListResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Should return threads around the anchor
			if len(response.Threads) == 0 {
				t.Error("Expected some threads in anchor query")
			}

			// Anchor query should have both anchors set
			if response.Pagination.BeforeAnchor == "" {
				t.Error("BeforeAnchor should be set for anchor query")
			}
			if response.Pagination.AfterAnchor == "" {
				t.Error("AfterAnchor should be set for anchor query")
			}
		})

		t.Run("Sort By - Created Timestamp", func(t *testing.T) {
			url := EndpointFrontendThreads + "?sort_by=created_ts"
			resp, err := DoRequest(t, "GET", url, nil, authHeaders)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			var response ThreadsListResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Verify threads are sorted by created_ts (newest first for initial load)
			for i := 1; i < len(response.Threads); i++ {
				if response.Threads[i-1].CreatedTS < response.Threads[i].CreatedTS {
					t.Errorf("Threads not sorted by created_ts (newest first): thread[%d].CreatedTS=%d, thread[%d].CreatedTS=%d",
						i-1, response.Threads[i-1].CreatedTS, i, response.Threads[i].CreatedTS)
				}
			}
		})

		t.Run("Sort By - Updated Timestamp", func(t *testing.T) {
			url := EndpointFrontendThreads + "?sort_by=updated_ts"
			resp, err := DoRequest(t, "GET", url, nil, authHeaders)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected status 200, got %d, body: %s", resp.StatusCode, string(body))
			}

			var response ThreadsListResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Verify threads are sorted by updated_ts (newest first for initial load)
			for i := 1; i < len(response.Threads); i++ {
				if response.Threads[i-1].UpdatedTS < response.Threads[i].UpdatedTS {
					t.Errorf("Threads not sorted by updated_ts (newest first): thread[%d].UpdatedTS=%d, thread[%d].UpdatedTS=%d",
						i-1, response.Threads[i-1].UpdatedTS, i, response.Threads[i].UpdatedTS)
				}
			}
		})

		t.Run("Invalid Parameters", func(t *testing.T) {
			testCases := []struct {
				name     string
				url      string
				expected int
			}{
				{"Invalid sort_by", EndpointFrontendThreads + "?sort_by=invalid", http.StatusBadRequest},
				{"Multiple reference points", EndpointFrontendThreads + "?before=t1&after=t2", http.StatusBadRequest},
				{"Limit too small", EndpointFrontendThreads + "?limit=0", http.StatusBadRequest},
				{"Limit too large", EndpointFrontendThreads + "?limit=1001", http.StatusBadRequest},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					resp, err := DoRequest(t, "GET", tc.url, nil, authHeaders)
					if err != nil {
						t.Fatalf("Request failed: %v", err)
					}
					defer resp.Body.Close()

					if resp.StatusCode != tc.expected {
						t.Errorf("Expected status %d, got %d", tc.expected, resp.StatusCode)
					}
				})
			}
		})

		t.Run("User Isolation", func(t *testing.T) {
			// User2 should only see their own threads
			resp, err := DoRequest(t, "GET", EndpointFrontendThreads, nil, user2Headers)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			var response ThreadsListResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Should only see user2's threads
			if len(response.Threads) != 3 {
				t.Errorf("Expected 3 threads for user2, got %d", len(response.Threads))
			}

			for _, thread := range response.Threads {
				if thread.Author != user2 {
					t.Errorf("User2 should not see threads from user1, but got thread with author: %s", thread.Author)
				}
			}
		})
	})
}

// TestReadThreadItem tests the ReadThreadItem endpoint
func TestReadThreadItem(t *testing.T) {
	WithTestServer(t, func() {
		user1 := "user_thread_item_test_1"
		user2 := "user_thread_item_test_2"

		// Get signed auth headers
		authHeaders1, err := SignedAuthHeaders(TestFrontendKey, user1)
		if err != nil {
			t.Fatalf("Failed to get signed auth headers: %v", err)
		}

		authHeaders2, err := SignedAuthHeaders(TestFrontendKey, user2)
		if err != nil {
			t.Fatalf("Failed to get signed auth headers: %v", err)
		}

		// Create a test thread for user1
		threadKeys := createTestThreads(t, authHeaders1, user1, 1)
		threadKey := threadKeys[0]

		t.Run("Owner Can Read Thread", func(t *testing.T) {
			url := EndpointFrontendThreads + "/" + threadKey
			resp, err := DoRequest(t, "GET", url, nil, authHeaders1)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			var response ThreadResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.Thread.Key != threadKey {
				t.Errorf("Expected thread key %s, got %s", threadKey, response.Thread.Key)
			}

			if response.Thread.Author != user1 {
				t.Errorf("Expected author %s, got %s", user1, response.Thread.Author)
			}
		})

		t.Run("Non-Owner Cannot Read Thread", func(t *testing.T) {
			url := EndpointFrontendThreads + "/" + threadKey
			resp, err := DoRequest(t, "GET", url, nil, authHeaders2)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 403, got %d", resp.StatusCode)
			}
		})

		t.Run("Invalid Thread Key", func(t *testing.T) {
			url := EndpointFrontendThreads + "/invalid_thread_key"
			resp, err := DoRequest(t, "GET", url, nil, authHeaders1)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			// Should return 404, 400, or 403 (all valid for invalid thread key)
			if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 404, 400, or 403, got %d", resp.StatusCode)
			}
		})
	})
}

// TestReadThreadMessages tests the ReadThreadMessages endpoint
func TestReadThreadMessages(t *testing.T) {
	WithTestServer(t, func() {
		user1 := "user_messages_test_1"
		user2 := "user_messages_test_2"

		// Get signed auth headers
		authHeaders1, err := SignedAuthHeaders(TestFrontendKey, user1)
		if err != nil {
			t.Fatalf("Failed to get signed auth headers: %v", err)
		}

		authHeaders2, err := SignedAuthHeaders(TestFrontendKey, user2)
		if err != nil {
			t.Fatalf("Failed to get signed auth headers: %v", err)
		}

		// Create a test thread with messages
		threadKeys := createTestThreads(t, authHeaders1, user1, 1)
		threadKey := threadKeys[0]

		// Add messages to the thread
		_ = createTestMessages(t, authHeaders1, threadKey, 5)

		t.Run("Owner Can Read Messages", func(t *testing.T) {
			url := ThreadMessagesURL(threadKey)
			resp, err := DoRequest(t, "GET", url, nil, authHeaders1)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			var response MessagesListResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.Thread != threadKey {
				t.Errorf("Expected thread key %s, got %s", threadKey, response.Thread)
			}

			if len(response.Messages) != 5 {
				t.Errorf("Expected 5 messages, got %d", len(response.Messages))
			}

			// Verify message ownership
			for _, message := range response.Messages {
				if message.Author != user1 {
					t.Errorf("Message author mismatch: expected %s, got %s", user1, message.Author)
				}
			}
		})

		t.Run("Non-Owner Cannot Read Messages", func(t *testing.T) {
			url := ThreadMessagesURL(threadKey)
			resp, err := DoRequest(t, "GET", url, nil, authHeaders2)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 403, got %d", resp.StatusCode)
			}
		})

		t.Run("Message Pagination", func(t *testing.T) {
			url := ThreadMessagesURL(threadKey) + "?limit=2"
			resp, err := DoRequest(t, "GET", url, nil, authHeaders1)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			var response MessagesListResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if len(response.Messages) != 2 {
				t.Errorf("Expected 2 messages, got %d", len(response.Messages))
			}

			if response.Pagination.Count != 2 {
				t.Errorf("Expected count 2, got %d", response.Pagination.Count)
			}

			// Should have HasAfter=false for initial load (even if more messages exist)
			if response.Pagination.HasAfter {
				t.Error("Should have HasAfter=false for initial load")
			}
		})
	})
}

// TestReadThreadMessage tests the ReadThreadMessage endpoint
func TestReadThreadMessage(t *testing.T) {
	WithTestServer(t, func() {
		user1 := "user_message_test_1"
		user2 := "user_message_test_2"

		// Get signed auth headers
		authHeaders1, err := SignedAuthHeaders(TestFrontendKey, user1)
		if err != nil {
			t.Fatalf("Failed to get signed auth headers: %v", err)
		}

		authHeaders2, err := SignedAuthHeaders(TestFrontendKey, user2)
		if err != nil {
			t.Fatalf("Failed to get signed auth headers: %v", err)
		}

		// Create a test thread with messages
		threadKeys := createTestThreads(t, authHeaders1, user1, 1)
		threadKey := threadKeys[0]

		messageKeys := createTestMessages(t, authHeaders1, threadKey, 1)
		messageKey := messageKeys[0]

		t.Run("Owner Can Read Message", func(t *testing.T) {
			url := ThreadMessagesURL(threadKey) + "/" + messageKey
			resp, err := DoRequest(t, "GET", url, nil, authHeaders1)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			var response MessageResponse
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Key gets resolved from provisional to final during processing
			// Just verify it starts with the provisional key
			if !strings.HasPrefix(response.Message.Key, messageKey) {
				t.Errorf("Expected message key to start with %s, got %s", messageKey, response.Message.Key)
			}

			if response.Message.Author != user1 {
				t.Errorf("Expected author %s, got %s", user1, response.Message.Author)
			}
		})

		t.Run("Non-Owner Cannot Read Message", func(t *testing.T) {
			url := ThreadMessagesURL(threadKey) + "/" + messageKey
			resp, err := DoRequest(t, "GET", url, nil, authHeaders2)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected status 403, got %d", resp.StatusCode)
			}
		})

		t.Run("Invalid Message Key", func(t *testing.T) {
			url := ThreadMessagesURL(threadKey) + "/invalid_message_key"
			resp, err := DoRequest(t, "GET", url, nil, authHeaders1)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			// Should return 404 or validation error
			if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected status 404 or 400, got %d", resp.StatusCode)
			}
		})
	})
}

// Helper functions for creating test data

// createTestThreads creates test threads for the given user
func createTestThreads(t *testing.T, authHeaders map[string]string, userID string, count int) []string {
	t.Helper()

	threadKeys := make([]string, 0, count)

	for i := 0; i < count; i++ {
		title := fmt.Sprintf("Test Thread %d for %s", i+1, userID)
		slug := MakeSlug(title, strconv.FormatInt(time.Now().UnixNano(), 10))

		threadData := map[string]interface{}{
			"title": title,
			"slug":  slug,
		}

		jsonBody, err := json.Marshal(threadData)
		if err != nil {
			t.Fatalf("Failed to marshal thread data: %v", err)
		}

		resp, err := DoRequest(t, "POST", EndpointFrontendThreads, jsonBody, authHeaders)
		if err != nil {
			t.Fatalf("Failed to create thread %d: %v", i+1, err)
		}
		defer resp.Body.Close()

		// Read body for error checking if needed
		body, _ := io.ReadAll(resp.Body)

		// Thread creation returns 202 (Accepted) for async processing
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			t.Fatalf("Failed to create thread %d: status %d, body: %s", i+1, resp.StatusCode, string(body))
		}

		// Handle both sync (200) and async (202) responses
		if resp.StatusCode == http.StatusAccepted {
			// For async responses, the response is just {"key":"thread_key"}
			var asyncResponse struct {
				Key string `json:"key"`
			}
			if err := json.Unmarshal(body, &asyncResponse); err != nil {
				t.Fatalf("Failed to decode async thread response: %v", err)
			}

			// Wait for async processing to complete
			time.Sleep(3 * time.Second)

			threadKeys = append(threadKeys, asyncResponse.Key)
			continue
		}

		var response ThreadResponse
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatalf("Failed to decode thread response: %v", err)
		}

		threadKeys = append(threadKeys, response.Thread.Key)
	}

	// Wait for async thread processing to complete
	time.Sleep(3 * time.Second)

	return threadKeys
}

// createTestMessages creates test messages in the given thread
func createTestMessages(t *testing.T, authHeaders map[string]string, threadKey string, count int) []string {
	t.Helper()

	messageKeys := make([]string, 0, count)

	for i := 0; i < count; i++ {
		content := fmt.Sprintf("Test message %d in thread %s", i+1, threadKey)

		messageData := map[string]interface{}{
			"body": map[string]interface{}{
				"type":    "text",
				"content": content,
			},
		}

		jsonBody, err := json.Marshal(messageData)
		if err != nil {
			t.Fatalf("Failed to marshal message data: %v", err)
		}

		resp, err := DoRequest(t, "POST", ThreadMessagesURL(threadKey), jsonBody, authHeaders)
		if err != nil {
			t.Fatalf("Failed to create message %d: %v", i+1, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Failed to create message %d: status %d, body: %s", i+1, resp.StatusCode, string(body))
		}

		var response struct {
			Key string `json:"key"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode message response: %v", err)
		}

		messageKeys = append(messageKeys, response.Key)
	}

	// Wait for async processing to complete before returning
	time.Sleep(2 * time.Second)

	return messageKeys
}

// Response types for testing
type ThreadsListResponse struct {
	Threads    []models.Thread                `json:"threads"`
	Pagination *pagination.PaginationResponse `json:"pagination"`
}

type ThreadResponse struct {
	Thread models.Thread `json:"thread"`
}

type MessagesListResponse struct {
	Thread     string                         `json:"thread"`
	Messages   []models.Message               `json:"messages"`
	Pagination *pagination.PaginationResponse `json:"pagination"`
}

type MessageResponse struct {
	Message models.Message `json:"message"`
}
