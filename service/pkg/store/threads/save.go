package threads

import (
	"fmt"

	"progressdb/pkg/logger"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// saves thread metadata as JSON
func SaveThread(threadID, data string) error {
	tr := telemetry.Track("storedb.save_thread")
	defer tr.Finish()

	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	tk, err := keys.ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	tr.Mark("set")
	if err := storedb.Client.Set([]byte(tk), []byte(data), storedb.WriteOpt(true)); err != nil {
		logger.Error("save_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_saved", "thread", threadID)
	return nil
}
