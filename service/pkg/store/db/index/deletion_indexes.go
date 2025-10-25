package index

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// UserSoftDeletedThreads holds soft deleted threads for a user.
type UserSoftDeletedThreads struct {
	Threads []string `json:"threads"`
}

// UserSoftDeletedMessages holds soft deleted messages for a user.
type UserSoftDeletedMessages struct {
	Messages []string `json:"messages"`
}

// UpdateSoftDeletedThreads adds or removes a soft deleted thread for user.
func UpdateSoftDeletedThreads(userID, threadID string, add bool) error {
	tr := telemetry.Track("index.update_soft_deleted_threads")
	defer tr.Finish()

	key := keys.GenSoftDeletedThreadsKey(userID)

	val, err := GetKey(key)
	if err != nil && !IsNotFound(err) {
		return err
	}

	var indexes UserSoftDeletedThreads
	if val != "" {
		if err := json.Unmarshal([]byte(val), &indexes); err != nil {
			return fmt.Errorf("unmarshal soft deleted threads: %w", err)
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
		return fmt.Errorf("marshal soft deleted threads: %w", err)
	}
	return SaveKey(key, data)
}

// UpdateSoftDeletedMessages adds or removes a soft deleted message for user.
func UpdateSoftDeletedMessages(userID, msgID string, add bool) error {
	tr := telemetry.Track("index.update_soft_deleted_messages")
	defer tr.Finish()

	key := keys.GenSoftDeletedMessagesKey(userID)

	val, err := GetKey(key)
	if err != nil && !IsNotFound(err) {
		return err
	}

	var indexes UserSoftDeletedMessages
	if val != "" {
		if err := json.Unmarshal([]byte(val), &indexes); err != nil {
			return fmt.Errorf("unmarshal soft deleted messages: %w", err)
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
		return fmt.Errorf("marshal soft deleted messages: %w", err)
	}
	return SaveKey(key, data)
}

// GetSoftDeletedThreads returns soft deleted threads for user.
func GetSoftDeletedThreads(userID string) ([]string, error) {
	key := keys.GenSoftDeletedThreadsKey(userID)

	val, err := GetKey(key)
	if err != nil {
		if IsNotFound(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var indexes UserSoftDeletedThreads
	if err := json.Unmarshal([]byte(val), &indexes); err != nil {
		return nil, fmt.Errorf("unmarshal soft deleted threads: %w", err)
	}
	return indexes.Threads, nil
}

// GetSoftDeletedMessages returns soft deleted messages for user.
func GetSoftDeletedMessages(userID string) ([]string, error) {
	key := keys.GenSoftDeletedMessagesKey(userID)

	val, err := GetKey(key)
	if err != nil {
		if IsNotFound(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var indexes UserSoftDeletedMessages
	if err := json.Unmarshal([]byte(val), &indexes); err != nil {
		return nil, fmt.Errorf("unmarshal soft deleted messages: %w", err)
	}
	return indexes.Messages, nil
}

// DeleteUserSoftDeletionIndexes removes user's soft deletion indexes.
func DeleteUserSoftDeletionIndexes(userID string) error {
	tr := telemetry.Track("index.delete_user_soft_deletion_indexes")
	defer tr.Finish()

	keysToDelete := []string{}
	keysToDelete = append(keysToDelete, keys.GenSoftDeletedThreadsKey(userID))
	keysToDelete = append(keysToDelete, keys.GenSoftDeletedMessagesKey(userID))

	for _, key := range keysToDelete {
		if err := DeleteKey(key); err != nil {
			logger.Error("delete_user_soft_deletion_index_failed", "key", key, "error", err)
			return err
		}
	}
	return nil
}
