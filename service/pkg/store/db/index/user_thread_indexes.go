package index

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// UserThreadIndexes holds threads owned by a user.
type UserThreadIndexes struct {
	Threads []string `json:"threads"`
}

// ThreadParticipantIndexes holds participants in a thread.
type ThreadParticipantIndexes struct {
	Participants []string `json:"participants"`
}

// UpdateUserOwnership adds or removes a thread from user's ownership.
func UpdateUserOwnership(userID, threadID string, add bool) error {
	tr := telemetry.Track("index.update_user_ownership")
	defer tr.Finish()

	key := keys.GenUserThreadsKey(userID)

	val, err := GetKey(key)
	if err != nil && !IsNotFound(err) {
		return err
	}

	var indexes UserThreadIndexes
	if val != "" {
		if err := json.Unmarshal([]byte(val), &indexes); err != nil {
			return fmt.Errorf("unmarshal user threads: %w", err)
		}
	}

	if add {
		// add if not present
		for _, t := range indexes.Threads {
			if t == threadID {
				return nil // already added
			}
		}
		indexes.Threads = append(indexes.Threads, threadID)
	} else {
		// remove
		for i, t := range indexes.Threads {
			if t == threadID {
				indexes.Threads = append(indexes.Threads[:i], indexes.Threads[i+1:]...)
				break
			}
		}
	}

	data, err := json.Marshal(indexes)
	if err != nil {
		return fmt.Errorf("marshal user threads: %w", err)
	}
	return SaveKey(key, data)
}

// UpdateThreadParticipants adds or removes a user from thread participants.
func UpdateThreadParticipants(threadID, userID string, add bool) error {
	tr := telemetry.Track("index.update_thread_participants")
	defer tr.Finish()

	key := keys.GenThreadParticipantsKey(threadID)

	val, err := GetKey(key)
	if err != nil && !IsNotFound(err) {
		return err
	}

	var indexes ThreadParticipantIndexes
	if val != "" {
		if err := json.Unmarshal([]byte(val), &indexes); err != nil {
			return fmt.Errorf("unmarshal thread participants: %w", err)
		}
	}

	if add {
		// add if not present
		for _, u := range indexes.Participants {
			if u == userID {
				return nil // already added
			}
		}
		indexes.Participants = append(indexes.Participants, userID)
	} else {
		// remove
		for i, u := range indexes.Participants {
			if u == userID {
				indexes.Participants = append(indexes.Participants[:i], indexes.Participants[i+1:]...)
				break
			}
		}
	}

	data, err := json.Marshal(indexes)
	if err != nil {
		return fmt.Errorf("marshal thread participants: %w", err)
	}
	return SaveKey(key, data)
}

// GetUserThreads returns threads owned by user.
func GetUserThreads(userID string) ([]string, error) {
	key := keys.GenUserThreadsKey(userID)

	val, err := GetKey(key)
	if err != nil {
		if IsNotFound(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var indexes UserThreadIndexes
	if err := json.Unmarshal([]byte(val), &indexes); err != nil {
		return nil, fmt.Errorf("unmarshal user threads: %w", err)
	}
	return indexes.Threads, nil
}

// ThreadWithTimestamp represents a thread with its creation timestamp
type ThreadWithTimestamp struct {
	ID        string
	Timestamp int64
}

// GetUserThreadsCursor returns threads owned by user with cursor-based pagination
func GetUserThreadsCursor(userID, cursor string, limit int) ([]string, string, bool, error) {
	tr := telemetry.Track("index.get_user_threads_cursor")
	defer tr.Finish()

	// Get all thread IDs for user
	threadIDs, err := GetUserThreads(userID)
	if err != nil {
		return nil, "", false, err
	}

	// Get thread metadata with timestamps
	threads := make([]ThreadWithTimestamp, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		threadKey := keys.GenThreadKey(threadID)
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
			ID:        threadID,
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
			if t.ID == tc.ThreadID && t.Timestamp == tc.Timestamp {
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
	threadIDsOnly := make([]string, len(pageThreads))
	for i, t := range pageThreads {
		threadIDsOnly[i] = t.ID
	}

	// Determine if there are more threads
	hasMore := endIndex < len(threads)

	// Generate next cursor if we have more
	var nextCursor string
	if hasMore && len(pageThreads) > 0 {
		lastThread := pageThreads[len(pageThreads)-1]
		nextCursor, err = encodeThreadCursor(userID, lastThread.ID, lastThread.Timestamp)
		if err != nil {
			return nil, "", false, err
		}
	}

	return threadIDsOnly, nextCursor, hasMore, nil
}

// Helper functions for cursor encoding/decoding
func encodeThreadCursor(userID, threadID string, timestamp int64) (string, error) {
	cursor := map[string]interface{}{
		"user_id":   userID,
		"thread_id": threadID,
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
	ThreadID  string `json:"thread_id"`
	Timestamp int64  `json:"timestamp"`
}, error) {
	var result struct {
		UserID    string `json:"user_id"`
		ThreadID  string `json:"thread_id"`
		Timestamp int64  `json:"timestamp"`
	}

	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	return result, err
}

// GetThreadParticipants returns participants in thread.
func GetThreadParticipants(threadID string) ([]string, error) {
	key := keys.GenThreadParticipantsKey(threadID)

	val, err := GetKey(key)
	if err != nil {
		if IsNotFound(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var indexes ThreadParticipantIndexes
	if err := json.Unmarshal([]byte(val), &indexes); err != nil {
		return nil, fmt.Errorf("unmarshal thread participants: %w", err)
	}
	return indexes.Participants, nil
}

// DeleteUserThreadIndexes removes user's thread ownership index.
func DeleteUserThreadIndexes(userID string) error {
	tr := telemetry.Track("index.delete_user_thread_indexes")
	defer tr.Finish()

	key := keys.GenUserThreadsKey(userID)
	if err := DeleteKey(key); err != nil {
		logger.Error("delete_user_thread_index_failed", "key", key, "error", err)
		return err
	}
	return nil
}

// DeleteThreadParticipantIndexes removes thread's participant index.
func DeleteThreadParticipantIndexes(threadID string) error {
	tr := telemetry.Track("index.delete_thread_participant_indexes")
	defer tr.Finish()

	key := keys.GenThreadParticipantsKey(threadID)
	if err := DeleteKey(key); err != nil {
		logger.Error("delete_thread_participant_index_failed", "key", key, "error", err)
		return err
	}
	return nil
}
