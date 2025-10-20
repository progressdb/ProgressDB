package threads

import (
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// deletes thread metadata
func DeleteThread(threadID string) error {
	tr := telemetry.Track("store.delete_thread")
	defer tr.Finish()

	if db.StoreDB == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := keys.ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	tr.Mark("delete")
	if err := db.StoreDB.Delete([]byte(tk), db.WriteOpt(true)); err != nil {
		logger.Error("delete_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_deleted", "thread", threadID)
	return nil
}

// marks thread as deleted: just sets Deleted field and saves
func SoftDeleteThread(threadID, actor string) error {
	tr := telemetry.Track("store.soft_delete_thread")
	defer tr.Finish()

	if db.StoreDB == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, terr := keys.ThreadMetaKey(threadID)
	if terr != nil {
		return terr
	}
	key := []byte(tk)
	tr.Mark("get_thread")
	v, closer, err := db.StoreDB.Get(key)
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
	if err := db.StoreDB.Set(key, nb, db.WriteOpt(true)); err != nil {
		logger.Error("soft_delete_save_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_soft_deleted", "thread", threadID, "actor", actor)
	return nil
}
