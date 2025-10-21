package index

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// ThreadMessageIndexes holds the index data for a thread's messages.
type ThreadMessageIndexes struct {
	Start         uint64   `json:"start"`
	End           uint64   `json:"end"`
	Cdeltas       []int64  `json:"cdeltas"`
	Udeltas       []int64  `json:"udeltas"`
	Skips         []string `json:"skips"`
	LastCreatedAt int64    `json:"last_created_at"`
	LastUpdatedAt int64    `json:"last_updated_at"`
}

// InitThreadMessageIndexes initializes the indexes for a new thread.
func InitThreadMessageIndexes(threadID string) error {
	tr := telemetry.Track("index.init_thread_message_indexes")
	defer tr.Finish()

	indexes := ThreadMessageIndexes{
		Start:         0,
		End:           0,
		Cdeltas:       []int64{},
		Udeltas:       []int64{},
		Skips:         []string{},
		LastCreatedAt: 0,
		LastUpdatedAt: 0,
	}

	return saveIndexes(threadID, indexes)
}

// UpdateOnMessageSave updates indexes when a message is saved.
func UpdateOnMessageSave(threadID string, createdAt, updatedAt int64) error {
	tr := telemetry.Track("index.update_on_message_save")
	defer tr.Finish()

	indexes, err := loadIndexes(threadID)
	if err != nil {
		return err
	}

	// Increment end
	indexes.End++

	// Append deltas
	createdDelta := createdAt - indexes.LastCreatedAt
	updatedDelta := updatedAt - indexes.LastUpdatedAt
	indexes.Cdeltas = append(indexes.Cdeltas, createdDelta)
	indexes.Udeltas = append(indexes.Udeltas, updatedDelta)

	// Update last timestamps
	indexes.LastCreatedAt = createdAt
	indexes.LastUpdatedAt = updatedAt

	return saveIndexes(threadID, indexes)
}

// UpdateOnMessageDelete updates indexes when a message is deleted.
func UpdateOnMessageDelete(threadID, msgKey string) error {
	tr := telemetry.Track("index.update_on_message_delete")
	defer tr.Finish()

	indexes, err := loadIndexes(threadID)
	if err != nil {
		return err
	}

	indexes.Skips = append(indexes.Skips, msgKey)

	return saveIndexes(threadID, indexes)
}

// DeleteThreadMessageIndexes removes all index keys for a thread.
func DeleteThreadMessageIndexes(threadID string) error {
	tr := telemetry.Track("index.delete_thread_message_indexes")
	defer tr.Finish()

	suffixes := []string{"start", "end", "cdeltas", "udeltas", "skips"}
	for _, suffix := range suffixes {
		key, err := keys.ThreadsToMessagesIndexKey(threadID, suffix)
		if err != nil {
			return err
		}
		if err := DeleteIndexKey(key); err != nil {
			logger.Error("delete_thread_message_index_failed", "key", key, "error", err)
			return err
		}
	}
	return nil
}

// GetIndexValue retrieves a single index value as a string.
func GetIndexValue(threadID, suffix string) (string, error) {
	key, err := keys.ThreadsToMessagesIndexKey(threadID, suffix)
	if err != nil {
		return "", err
	}
	return GetIndexKey(key)
}

// GetThreadMessageIndexes loads all indexes for a thread.
func GetThreadMessageIndexes(threadID string) (ThreadMessageIndexes, error) {
	return loadIndexes(threadID)
}

// loadIndexes loads the indexes from the DB.
func loadIndexes(threadID string) (ThreadMessageIndexes, error) {
	var indexes ThreadMessageIndexes

	// Load each field
	fields := map[string]interface{}{
		"start":           &indexes.Start,
		"end":             &indexes.End,
		"cdeltas":         &indexes.Cdeltas,
		"udeltas":         &indexes.Udeltas,
		"skips":           &indexes.Skips,
		"last_created_at": &indexes.LastCreatedAt,
		"last_updated_at": &indexes.LastUpdatedAt,
	}

	for suffix, ptr := range fields {
		key, err := keys.ThreadsToMessagesIndexKey(threadID, suffix)
		if err != nil {
			return indexes, err
		}
		val, err := GetIndexKey(key)
		if err != nil {
			if IndexIsNotFound(err) {
				// If not found, assume default (for new threads or missing fields)
				continue
			}
			return indexes, err
		}
		if err := json.Unmarshal([]byte(val), ptr); err != nil {
			return indexes, fmt.Errorf("unmarshal index %s: %w", suffix, err)
		}
	}

	return indexes, nil
}

// saveIndexes saves the indexes to the DB.
func saveIndexes(threadID string, indexes ThreadMessageIndexes) error {
	fields := map[string]interface{}{
		"start":           indexes.Start,
		"end":             indexes.End,
		"cdeltas":         indexes.Cdeltas,
		"udeltas":         indexes.Udeltas,
		"skips":           indexes.Skips,
		"last_created_at": indexes.LastCreatedAt,
		"last_updated_at": indexes.LastUpdatedAt,
	}

	for suffix, val := range fields {
		key, err := keys.ThreadsToMessagesIndexKey(threadID, suffix)
		if err != nil {
			return err
		}
		data, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("marshal index %s: %w", suffix, err)
		}
		if err := SaveIndexKey(key, data); err != nil {
			return err
		}
	}
	return nil
}
