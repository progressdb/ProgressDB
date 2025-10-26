package index

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// GetUserThreads returns threads owned by user using relationship key scanning.
func GetUserThreads(userID string) ([]string, error) {
	prefix := fmt.Sprintf("rel:u:%s:t:", userID)
	keys, err := ListKeys(prefix)
	if err != nil {
		return nil, fmt.Errorf("list user ownership keys: %w", err)
	}

	// Extract thread IDs from keys like "rel:u:<user_id>:t:<thread_id>"
	threads := make([]string, 0, len(keys))
	for _, key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) >= 5 && parts[0] == "rel" && parts[1] == "u" && parts[2] == userID && parts[3] == "t" {
			threads = append(threads, parts[4])
		}
	}
	return threads, nil
}

// ThreadWithTimestamp represents a thread with its creation timestamp
type ThreadWithTimestamp struct {
	Key       string
	Timestamp int64
}

// GetUserThreadsCursor returns threads owned by user with cursor-based pagination
func GetUserThreadsCursor(userID, cursor string, limit int) ([]string, string, bool, error) {
	tr := telemetry.Track("index.get_user_threads_cursor")
	defer tr.Finish()

	// Get all thread IDs for user
	threadKeys, err := GetUserThreads(userID)
	if err != nil {
		return nil, "", false, err
	}

	// Get thread metadata with timestamps
	threads := make([]ThreadWithTimestamp, 0, len(threadKeys))
	for _, threadKey := range threadKeys {
		threadKey := keys.GenThreadKey(threadKey)
		threadData, err := GetKey(threadKey)
		if err != nil {
			continue // Skip threads that can't be found
		}

		var threadMeta struct {
			CreatedAt int64 `json:"created_at"`
		}
		if err := json.Unmarshal([]byte(threadData), &threadMeta); err != nil {
			continue // Skip invalid thread metadata
		}

		threads = append(threads, ThreadWithTimestamp{
			Key:       threadKey,
			Timestamp: threadMeta.CreatedAt,
		})
	}

	// Sort by timestamp (newest first)
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].Timestamp > threads[j].Timestamp
	})

	// Find starting position from cursor
	startIndex := 0
	if cursor != "" {
		tc, err := decodeThreadCursor(cursor)
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor: %w", err)
		}
		if tc.UserID != userID {
			return nil, "", false, fmt.Errorf("cursor user mismatch")
		}

		// Find the thread with the cursor timestamp
		for i, t := range threads {
			if t.Key == tc.ThreadKey && t.Timestamp == tc.Timestamp {
				startIndex = i + 1 // Start after the cursor position
				break
			}
		}
	}

	// Extract the page
	endIndex := startIndex + limit
	if endIndex > len(threads) {
		endIndex = len(threads)
	}

	if startIndex >= len(threads) {
		return []string{}, "", false, nil
	}

	pageThreads := threads[startIndex:endIndex]
	threadKeysOnly := make([]string, len(pageThreads))
	for i, t := range pageThreads {
		threadKeysOnly[i] = t.Key
	}

	// Determine if there are more threads
	hasMore := endIndex < len(threads)

	// Generate next cursor if we have more
	var nextCursor string
	if hasMore && len(pageThreads) > 0 {
		lastThread := pageThreads[len(pageThreads)-1]
		nextCursor, err = encodeThreadCursor(userID, lastThread.Key, lastThread.Timestamp)
		if err != nil {
			return nil, "", false, err
		}
	}

	return threadKeysOnly, nextCursor, hasMore, nil
}

// Helper functions for cursor encoding/decoding
func encodeThreadCursor(userID, threadKey string, timestamp int64) (string, error) {
	cursor := map[string]interface{}{
		"user_id":   userID,
		"thread_id": threadKey,
		"timestamp": timestamp,
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func decodeThreadCursor(cursor string) (struct {
	UserID    string `json:"user_id"`
	ThreadKey string `json:"thread_key"`
	Timestamp int64  `json:"timestamp"`
}, error) {
	var result struct {
		UserID    string `json:"user_id"`
		ThreadKey string `json:"thread_key"`
		Timestamp int64  `json:"timestamp"`
	}

	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	return result, err
}

// GetThreadParticipants returns participants in thread using relationship key scanning.
func GetThreadParticipants(threadKey string) ([]string, error) {
	prefix := fmt.Sprintf("rel:t:%s:u:", threadKey)
	keys, err := ListKeys(prefix)
	if err != nil {
		return nil, fmt.Errorf("list thread participant keys: %w", err)
	}

	// Extract user IDs from keys like "rel:t:<thread_id>:u:<user_id>"
	users := make([]string, 0, len(keys))
	for _, key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) >= 5 && parts[0] == "rel" && parts[1] == "t" && parts[2] == threadKey && parts[3] == "u" {
			users = append(users, parts[4])
		}
	}
	return users, nil
}
