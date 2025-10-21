package index

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// UserDeletedThreads holds deleted threads for a user.
type UserDeletedThreads struct {
	Threads []string `json:"threads"`
}

// UserDeletedMessages holds deleted messages for a user.
type UserDeletedMessages struct {
	Messages []string `json:"messages"`
}

// UpdateDeletedThreads adds or removes a deleted thread for user.
func UpdateDeletedThreads(userID, threadID string, add bool) error {
	tr := telemetry.Track("index.update_deleted_threads")
	defer tr.Finish()

	key, err := keys.DeletedThreadsIndexKey(userID)
	if err != nil {
		return err
	}

	val, err := GetIndexKey(key)
	if err != nil && !IndexIsNotFound(err) {
		return err
	}

	var indexes UserDeletedThreads
	if val != "" {
		if err := json.Unmarshal([]byte(val), &indexes); err != nil {
			return fmt.Errorf("unmarshal deleted threads: %w", err)
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
		return fmt.Errorf("marshal deleted threads: %w", err)
	}
	return SaveIndexKey(key, data)
}

// UpdateDeletedMessages adds or removes a deleted message for user.
func UpdateDeletedMessages(userID, msgID string, add bool) error {
	tr := telemetry.Track("index.update_deleted_messages")
	defer tr.Finish()

	key, err := keys.DeletedMessagesIndexKey(userID)
	if err != nil {
		return err
	}

	val, err := GetIndexKey(key)
	if err != nil && !IndexIsNotFound(err) {
		return err
	}

	var indexes UserDeletedMessages
	if val != "" {
		if err := json.Unmarshal([]byte(val), &indexes); err != nil {
			return fmt.Errorf("unmarshal deleted messages: %w", err)
		}
	}

	if add {
		// add if not present
		for _, m := range indexes.Messages {
			if m == msgID {
				return nil // already added
			}
		}
		indexes.Messages = append(indexes.Messages, msgID)
	} else {
		// remove
		for i, m := range indexes.Messages {
			if m == msgID {
				indexes.Messages = append(indexes.Messages[:i], indexes.Messages[i+1:]...)
				break
			}
		}
	}

	data, err := json.Marshal(indexes)
	if err != nil {
		return fmt.Errorf("marshal deleted messages: %w", err)
	}
	return SaveIndexKey(key, data)
}

// GetDeletedThreads returns deleted threads for user.
func GetDeletedThreads(userID string) ([]string, error) {
	key, err := keys.DeletedThreadsIndexKey(userID)
	if err != nil {
		return nil, err
	}

	val, err := GetIndexKey(key)
	if err != nil {
		if IndexIsNotFound(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var indexes UserDeletedThreads
	if err := json.Unmarshal([]byte(val), &indexes); err != nil {
		return nil, fmt.Errorf("unmarshal deleted threads: %w", err)
	}
	return indexes.Threads, nil
}

// GetDeletedMessages returns deleted messages for user.
func GetDeletedMessages(userID string) ([]string, error) {
	key, err := keys.DeletedMessagesIndexKey(userID)
	if err != nil {
		return nil, err
	}

	val, err := GetIndexKey(key)
	if err != nil {
		if IndexIsNotFound(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var indexes UserDeletedMessages
	if err := json.Unmarshal([]byte(val), &indexes); err != nil {
		return nil, fmt.Errorf("unmarshal deleted messages: %w", err)
	}
	return indexes.Messages, nil
}

// DeleteUserDeletionIndexes removes user's deletion indexes.
func DeleteUserDeletionIndexes(userID string) error {
	tr := telemetry.Track("index.delete_user_deletion_indexes")
	defer tr.Finish()

	keysToDelete := []string{}
	if k, err := keys.DeletedThreadsIndexKey(userID); err == nil {
		keysToDelete = append(keysToDelete, k)
	}
	if k, err := keys.DeletedMessagesIndexKey(userID); err == nil {
		keysToDelete = append(keysToDelete, k)
	}

	for _, key := range keysToDelete {
		if err := DeleteIndexKey(key); err != nil {
			logger.Error("delete_user_deletion_index_failed", "key", key, "error", err)
			return err
		}
	}
	return nil
}
