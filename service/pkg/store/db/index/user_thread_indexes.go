package index

import (
	"encoding/json"
	"fmt"

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

	key, err := keys.UserThreadsIndexKey(userID)
	if err != nil {
		return err
	}

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

	key, err := keys.ThreadParticipantsIndexKey(threadID)
	if err != nil {
		return err
	}

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
	key, err := keys.UserThreadsIndexKey(userID)
	if err != nil {
		return nil, err
	}

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

// GetThreadParticipants returns participants in thread.
func GetThreadParticipants(threadID string) ([]string, error) {
	key, err := keys.ThreadParticipantsIndexKey(threadID)
	if err != nil {
		return nil, err
	}

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

	key, err := keys.UserThreadsIndexKey(userID)
	if err != nil {
		return err
	}
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

	key, err := keys.ThreadParticipantsIndexKey(threadID)
	if err != nil {
		return err
	}
	if err := DeleteKey(key); err != nil {
		logger.Error("delete_thread_participant_index_failed", "key", key, "error", err)
		return err
	}
	return nil
}
