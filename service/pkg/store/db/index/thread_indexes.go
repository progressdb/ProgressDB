package index

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

type ThreadMessageIndexes struct {
	Start         uint64   `json:"start"`
	End           uint64   `json:"end"`
	Cdeltas       []int64  `json:"cdeltas"`
	Udeltas       []int64  `json:"udeltas"`
	Skips         []string `json:"skips"`
	LastCreatedAt int64    `json:"last_created_at"`
	LastUpdatedAt int64    `json:"last_updated_at"`
}

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

func UpdateOnMessageSave(threadID string, createdAt, updatedAt int64) error {
	tr := telemetry.Track("index.update_on_message_save")
	defer tr.Finish()

	indexes, err := loadIndexes(threadID)
	if err != nil {
		return err
	}

	indexes.End++

	createdDelta := createdAt - indexes.LastCreatedAt
	updatedDelta := updatedAt - indexes.LastUpdatedAt
	indexes.Cdeltas = append(indexes.Cdeltas, createdDelta)
	indexes.Udeltas = append(indexes.Udeltas, updatedDelta)

	indexes.LastCreatedAt = createdAt
	indexes.LastUpdatedAt = updatedAt

	return saveIndexes(threadID, indexes)
}

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

func DeleteThreadMessageIndexes(threadID string) error {
	tr := telemetry.Track("index.delete_thread_message_indexes")
	defer tr.Finish()

	suffixes := []string{"start", "end", "cdeltas", "udeltas", "skips"}
	for _, suffix := range suffixes {
		var key string
		switch suffix {
		case "start":
			key = keys.GenThreadMessageStart(threadID)
		case "end":
			key = keys.GenThreadMessageEnd(threadID)
		case "cdeltas":
			key = keys.GenThreadMessageCDeltas(threadID)
		case "udeltas":
			key = keys.GenThreadMessageUDeltas(threadID)
		case "skips":
			key = keys.GenThreadMessageSkips(threadID)
		}
		if err := DeleteKey(key); err != nil {
			logger.Error("delete_thread_message_index_failed", "key", key, "error", err)
			return err
		}
	}
	return nil
}

func GetIndexValue(threadID, suffix string) (string, error) {
	var key string
	switch suffix {
	case "start":
		key = keys.GenThreadMessageStart(threadID)
	case "end":
		key = keys.GenThreadMessageEnd(threadID)
	case "cdeltas":
		key = keys.GenThreadMessageCDeltas(threadID)
	case "udeltas":
		key = keys.GenThreadMessageUDeltas(threadID)
	case "skips":
		key = keys.GenThreadMessageSkips(threadID)
	case "last_created_at":
		key = keys.GenThreadMessageLC(threadID)
	case "last_updated_at":
		key = keys.GenThreadMessageLU(threadID)
	default:
		return "", fmt.Errorf("unknown index suffix: %s", suffix)
	}
	return GetKey(key)
}

func GetThreadMessageIndexes(threadID string) (ThreadMessageIndexes, error) {
	return loadIndexes(threadID)
}

func loadIndexes(threadID string) (ThreadMessageIndexes, error) {
	var indexes ThreadMessageIndexes

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
		var key string
		switch suffix {
		case "start":
			key = keys.GenThreadMessageStart(threadID)
		case "end":
			key = keys.GenThreadMessageEnd(threadID)
		case "cdeltas":
			key = keys.GenThreadMessageCDeltas(threadID)
		case "udeltas":
			key = keys.GenThreadMessageUDeltas(threadID)
		case "skips":
			key = keys.GenThreadMessageSkips(threadID)
		case "last_created_at":
			key = keys.GenThreadMessageLC(threadID)
		case "last_updated_at":
			key = keys.GenThreadMessageLU(threadID)
		default:
			return indexes, fmt.Errorf("unknown index suffix: %s", suffix)
		}
		val, err := GetKey(key)
		if err != nil {
			if IsNotFound(err) {
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
		var key string
		switch suffix {
		case "start":
			key = keys.GenThreadMessageStart(threadID)
		case "end":
			key = keys.GenThreadMessageEnd(threadID)
		case "cdeltas":
			key = keys.GenThreadMessageCDeltas(threadID)
		case "udeltas":
			key = keys.GenThreadMessageUDeltas(threadID)
		case "skips":
			key = keys.GenThreadMessageSkips(threadID)
		case "last_created_at":
			key = keys.GenThreadMessageLC(threadID)
		case "last_updated_at":
			key = keys.GenThreadMessageLU(threadID)
		default:
			return fmt.Errorf("unknown index suffix: %s", suffix)
		}
		data, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("marshal index %s: %w", suffix, err)
		}
		if err := SaveKey(key, data); err != nil {
			return err
		}
	}
	return nil
}
