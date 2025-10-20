package threads

import (
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/db"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// saves thread metadata as JSON
func SaveThread(threadID, data string) error {
	tr := telemetry.Track("store.save_thread")
	defer tr.Finish()

	if db.StoreDB == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := keys.ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	tr.Mark("set")
	if err := db.StoreDB.Set([]byte(tk), []byte(data), db.WriteOpt(true)); err != nil {
		logger.Error("save_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_saved", "thread", threadID)
	return nil
}
