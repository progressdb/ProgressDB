package index

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/state/telemetry"
	"progressdb/pkg/store/keys"
)

func UpdateVersionIndexes(threadID, msgID string, ts int64, seq uint64, createdAt, updatedAt int64) error {
	tr := telemetry.Track("index.update_version_indexes")
	defer tr.Finish()

	suffixes := []string{"start", "end", "cdeltas", "udeltas", "skips", "last_created_at", "last_updated_at"}

	var indexes ThreadMessageIndexes
	for _, s := range suffixes {
		var key string
		switch s {
		case "start":
			key = keys.GenThreadVersionStart(threadID, msgID)
		case "end":
			key = keys.GenThreadVersionEnd(threadID, msgID)
		case "cdeltas":
			key = keys.GenThreadVersionCDeltas(threadID, msgID)
		case "udeltas":
			key = keys.GenThreadVersionUDeltas(threadID, msgID)
		case "skips":
			key = keys.GenThreadVersionSkips(threadID, msgID)
		case "last_created_at":
			key = keys.GenThreadVersionLC(threadID, msgID)
		case "last_updated_at":
			key = keys.GenThreadVersionLU(threadID, msgID)
		}
		val, err := GetKey(key)
		if err != nil {
			if IsNotFound(err) {
				continue
			}
			return err
		}
		var ptr interface{}
		switch s {
		case "start":
			ptr = &indexes.Start
		case "end":
			ptr = &indexes.End
		case "cdeltas":
			ptr = &indexes.Cdeltas
		case "udeltas":
			ptr = &indexes.Udeltas
		case "skips":
			ptr = &indexes.Skips
		case "last_created_at":
			ptr = &indexes.LastCreatedAt
		case "last_updated_at":
			ptr = &indexes.LastUpdatedAt
		}
		if err := json.Unmarshal([]byte(val), ptr); err != nil {
			return fmt.Errorf("unmarshal version index %s: %w", s, err)
		}
	}

	indexes.End++
	createdDelta := createdAt - indexes.LastCreatedAt
	updatedDelta := updatedAt - indexes.LastUpdatedAt
	indexes.Cdeltas = append(indexes.Cdeltas, createdDelta)
	indexes.Udeltas = append(indexes.Udeltas, updatedDelta)
	indexes.LastCreatedAt = createdAt
	indexes.LastUpdatedAt = updatedAt

	for _, s := range suffixes {
		var key string
		switch s {
		case "start":
			key = keys.GenThreadVersionStart(threadID, msgID)
		case "end":
			key = keys.GenThreadVersionEnd(threadID, msgID)
		case "cdeltas":
			key = keys.GenThreadVersionCDeltas(threadID, msgID)
		case "udeltas":
			key = keys.GenThreadVersionUDeltas(threadID, msgID)
		case "skips":
			key = keys.GenThreadVersionSkips(threadID, msgID)
		case "last_created_at":
			key = keys.GenThreadVersionLC(threadID, msgID)
		case "last_updated_at":
			key = keys.GenThreadVersionLU(threadID, msgID)
		}
		var val interface{}
		switch s {
		case "start":
			val = indexes.Start
		case "end":
			val = indexes.End
		case "cdeltas":
			val = indexes.Cdeltas
		case "udeltas":
			val = indexes.Udeltas
		case "skips":
			val = indexes.Skips
		case "last_created_at":
			val = indexes.LastCreatedAt
		case "last_updated_at":
			val = indexes.LastUpdatedAt
		}
		data, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("marshal version index %s: %w", s, err)
		}
		if err := SaveKey(key, data); err != nil {
			return err
		}
	}
	return nil
}

func DeleteVersionIndexes(threadID, msgID string) error {
	tr := telemetry.Track("index.delete_version_indexes")
	defer tr.Finish()

	suffixes := []string{"start", "end", "cdeltas", "udeltas", "skips", "last_created_at", "last_updated_at"}

	for _, s := range suffixes {
		var key string
		switch s {
		case "start":
			key = keys.GenThreadVersionStart(threadID, msgID)
		case "end":
			key = keys.GenThreadVersionEnd(threadID, msgID)
		case "cdeltas":
			key = keys.GenThreadVersionCDeltas(threadID, msgID)
		case "udeltas":
			key = keys.GenThreadVersionUDeltas(threadID, msgID)
		case "skips":
			key = keys.GenThreadVersionSkips(threadID, msgID)
		case "last_created_at":
			key = keys.GenThreadVersionLC(threadID, msgID)
		case "last_updated_at":
			key = keys.GenThreadVersionLU(threadID, msgID)
		}
		if err := DeleteKey(key); err != nil {
			logger.Error("delete_version_index_failed", "key", key, "error", err)
			return err
		}
	}
	return nil
}
