package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAdminStatsOptimization(t *testing.T) {
	WithTestServer(t, func() {
		// Get signed auth headers for frontend requests
		authHeaders1, err := SignedAuthHeaders(TestFrontendKey, "user1")
		assert.NoError(t, err)

		authHeaders2, err := SignedAuthHeaders(TestFrontendKey, "user2")
		assert.NoError(t, err)

		// Create test threads
		threadKeys1 := createTestThreads(t, authHeaders1, "user1", 2)
		threadKeys2 := createTestThreads(t, authHeaders2, "user2", 1)

		// Add messages to threads using existing function
		for _, threadKey := range threadKeys1 {
			createTestMessages(t, authHeaders1, threadKey, 3)
		}

		for _, threadKey := range threadKeys2 {
			createTestMessages(t, authHeaders2, threadKey, 2)
		}

		// Wait for async processing
		time.Sleep(3 * time.Second)

		// Test the optimized stats endpoint
		baseURL := strings.TrimSuffix(EndpointAdminHealth, "/health")
		statsURL := baseURL + "/stats"

		resp, err := DoRequest(t, "GET", statsURL, nil, AuthHeaders(TestAdminKey))
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)

		var statsResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&statsResponse)
		assert.NoError(t, err)

		// Verify the response structure
		assert.Contains(t, statsResponse, "threads")
		assert.Contains(t, statsResponse, "messages")

		threads := statsResponse["threads"].(float64)
		messages := statsResponse["messages"].(float64)

		// Should have 3 threads and 8 messages total (2*3 + 1*2)
		assert.Equal(t, float64(3), threads)
		assert.Equal(t, float64(8), messages)

		// Test performance by making multiple requests
		for i := 0; i < 5; i++ {
			resp, err := DoRequest(t, "GET", statsURL, nil, AuthHeaders(TestAdminKey))
			assert.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, 200, resp.StatusCode)
		}
	})
}
