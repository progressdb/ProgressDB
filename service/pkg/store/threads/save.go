package threads

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
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

	// Initialize thread message indexes for new thread
	if err := index.InitThreadMessageIndexes(threadID); err != nil {
		logger.Error("init_thread_message_indexes_failed", "thread", threadID, "error", err)
		return err
	}

	// Update user ownership
	var th models.Thread
	if err := json.Unmarshal([]byte(data), &th); err != nil {
		logger.Error("unmarshal_thread_for_ownership_failed", "thread", threadID, "error", err)
		return err
	}
	if th.Author != "" {
		if err := index.UpdateUserOwnership(th.Author, threadID, true); err != nil {
			logger.Error("update_user_ownership_failed", "user", th.Author, "thread", threadID, "error", err)
			return err
		}
	}

	return nil
}
