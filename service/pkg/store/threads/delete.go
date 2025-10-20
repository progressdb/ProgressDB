package threads

import (
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// deletes thread metadata
func DeleteThread(threadID string) error {
	tr := telemetry.Track("storedb.delete_thread")
	defer tr.Finish()

	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	tk, err := keys.ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	tr.Mark("delete")
	if err := storedb.Client.Delete([]byte(tk), storedb.WriteOpt(true)); err != nil {
		logger.Error("delete_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_deleted", "thread", threadID)
	return nil
}

// marks thread as deleted: just sets Deleted field and saves
func SoftDeleteThread(threadID, actor string) error {
	tr := telemetry.Track("storedb.soft_delete_thread")
	defer tr.Finish()

	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	tk, terr := keys.ThreadMetaKey(threadID)
	if terr != nil {
		return terr
	}
	key := []byte(tk)
	tr.Mark("get_thread")
	v, closer, err := storedb.Client.Get(key)
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
	if err := storedb.Client.Set(key, nb, storedb.WriteOpt(true)); err != nil {
		logger.Error("soft_delete_save_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_soft_deleted", "thread", threadID, "actor", actor)
	return nil
}
