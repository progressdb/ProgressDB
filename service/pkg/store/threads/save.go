package store

import (
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

// saves thread metadata as JSON
func SaveThread(threadID, data string) error {
	tr := telemetry.Track("store.save_thread")
	defer tr.Finish()

	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	tr.Mark("set")
	if err := db.Set([]byte(tk), []byte(data), writeOpt(true)); err != nil {
		logger.Error("save_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_saved", "thread", threadID)
	return nil
}
