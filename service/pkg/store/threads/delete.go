package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

// deletes thread metadata
func DeleteThread(threadID string) error {
	tr := telemetry.Track("store.delete_thread")
	defer tr.Finish()

	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	tr.Mark("delete")
	if err := db.Delete([]byte(tk), writeOpt(true)); err != nil {
		logger.Error("delete_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_deleted", "thread", threadID)
	return nil
}

// marks thread as deleted and adds a tombstone message
func SoftDeleteThread(threadID, actor string) error {
	tr := telemetry.Track("store.soft_delete_thread")
	defer tr.Finish()

	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, terr := ThreadMetaKey(threadID)
	if terr != nil {
		return terr
	}
	key := []byte(tk)
	tr.Mark("get_thread")
	v, closer, err := db.Get(key)
	if err != nil {
		logger.Error("soft_delete_load_failed", "thread", threadID, "error", err)
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	var th models.Thread
	if err := json.Unmarshal(v, &th); err != nil {
		logger.Error("soft_delete_unmarshal_failed", "thread", threadID, "error", err)
		return err
	}
	th.Deleted = true
	th.DeletedTS = time.Now().UTC().UnixNano()
	nb, _ := json.Marshal(th)
	tr.Mark("update_thread")
	if err := db.Set(key, nb, writeOpt(true)); err != nil {
		logger.Error("soft_delete_save_failed", "thread", threadID, "error", err)
		return err
	}
	tomb := models.Message{
		ID:      GenMessageID(),
		Thread:  threadID,
		Author:  actor,
		TS:      time.Now().UTC().UnixNano(),
		Body:    map[string]interface{}{"_event": "thread_deleted", "by": actor},
		Deleted: true,
	}
	// use background context
	tr.Mark("save_tombstone")
	if err := SaveMessage(context.Background(), threadID, tomb.ID, tomb); err != nil {
		logger.Error("soft_delete_append_tombstone_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_soft_deleted", "thread", threadID, "actor", actor)
	return nil
}
