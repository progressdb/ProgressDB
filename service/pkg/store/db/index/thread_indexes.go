package index

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/state/telemetry"
	"progressdb/pkg/store/keys"
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

func InitThreadMessageIndexes(threadKey string) error {
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

	return saveIndexes(threadKey, indexes)
}

func DeleteThreadMessageIndexes(threadKey string) error {
	tr := telemetry.Track("index.delete_thread_message_indexes")
	defer tr.Finish()

	suffixes := []string{"start", "end", "cdeltas", "udeltas", "skips"}
	for _, suffix := range suffixes {
		var key string
		switch suffix {
		case "start":
			key = keys.GenThreadMessageStart(threadKey)
		case "end":
			key = keys.GenThreadMessageEnd(threadKey)
		case "cdeltas":
			key = keys.GenThreadMessageCDeltas(threadKey)
		case "udeltas":
			key = keys.GenThreadMessageUDeltas(threadKey)
		case "skips":
			key = keys.GenThreadMessageSkips(threadKey)
		}
		if err := DeleteKey(key); err != nil {
			logger.Error("delete_thread_message_index_failed", "key", key, "error", err)
			return err
		}
	}
	return nil
}

func GetThreadIndexValue(threadKey, suffix string) (string, error) {
	var key string
	switch suffix {
	case "start":
		key = keys.GenThreadMessageStart(threadKey)
	case "end":
		key = keys.GenThreadMessageEnd(threadKey)
	case "cdeltas":
		key = keys.GenThreadMessageCDeltas(threadKey)
	case "udeltas":
		key = keys.GenThreadMessageUDeltas(threadKey)
	case "skips":
		key = keys.GenThreadMessageSkips(threadKey)
	case "last_created_at":
		key = keys.GenThreadMessageLC(threadKey)
	case "last_updated_at":
		key = keys.GenThreadMessageLU(threadKey)
	default:
		return "", fmt.Errorf("unknown index suffix: %s", suffix)
	}
	return GetKey(key)
}

func GetThreadMessageIndexes(threadKey string) (ThreadMessageIndexes, error) {
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
			key = keys.GenThreadMessageStart(threadKey)
		case "end":
			key = keys.GenThreadMessageEnd(threadKey)
		case "cdeltas":
			key = keys.GenThreadMessageCDeltas(threadKey)
		case "udeltas":
			key = keys.GenThreadMessageUDeltas(threadKey)
		case "skips":
			key = keys.GenThreadMessageSkips(threadKey)
		case "last_created_at":
			key = keys.GenThreadMessageLC(threadKey)
		case "last_updated_at":
			key = keys.GenThreadMessageLU(threadKey)
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

func saveIndexes(threadKey string, indexes ThreadMessageIndexes) error {
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
			key = keys.GenThreadMessageStart(threadKey)
		case "end":
			key = keys.GenThreadMessageEnd(threadKey)
		case "cdeltas":
			key = keys.GenThreadMessageCDeltas(threadKey)
		case "udeltas":
			key = keys.GenThreadMessageUDeltas(threadKey)
		case "skips":
			key = keys.GenThreadMessageSkips(threadKey)
		case "last_created_at":
			key = keys.GenThreadMessageLC(threadKey)
		case "last_updated_at":
			key = keys.GenThreadMessageLU(threadKey)
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
