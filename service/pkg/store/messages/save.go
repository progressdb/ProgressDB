package messages

import (
	"context"
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/locks"
	"progressdb/pkg/telemetry"
)

// saves message with encryption and sequencing
func SaveMessage(ctx context.Context, threadID, msgID string, msg models.Message, threadJSON string) error {
	tr := telemetry.Track("storedb.save_message")
	defer tr.Finish()

	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}

	// lock thread for sequencing
	l := locks.GetThreadLock(threadID)
	l.Lock()
	defer l.Unlock()

	// compute next sequence
	seq, err := locks.ComputeMaxSeq(threadID)
	if err != nil {
		return fmt.Errorf("failed to compute max seq: %w", err)
	}
	seq++

	// build keys
	msgKey, err := keys.MsgKey(threadID, msg.TS, seq)
	if err != nil {
		return fmt.Errorf("invalid msg key: %w", err)
	}
	versionKey, err := keys.VersionKey(msgID, msg.TS, seq)
	if err != nil {
		return fmt.Errorf("invalid version key: %w", err)
	}

	// marshal message
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// encrypt if enabled
	var thread models.Thread
	if err := json.Unmarshal([]byte(threadJSON), &thread); err != nil {
		return fmt.Errorf("failed to unmarshal thread: %w", err)
	}
	data, err = encryption.EncryptMessageData(&thread, data)
	if err != nil {
		return fmt.Errorf("failed to encrypt message: %w", err)
	}

	// save to batch
	batch := storedb.Client.NewBatch()
	defer batch.Close()
	batch.Set([]byte(msgKey), data, storedb.WriteOpt(true))
	batch.Set([]byte(versionKey), data, storedb.WriteOpt(true))

	tr.Mark("apply")
	if err := storedb.Client.Apply(batch, storedb.WriteOpt(true)); err != nil {
		logger.Error("save_message_failed", "thread", threadID, "msg", msgID, "error", err)
		return err
	}

	logger.Info("message_saved", "thread", threadID, "msg", msgID, "seq", seq)

	// Update thread message indexes
	if msg.Deleted {
		// On delete, add to skips
		if err := index.UpdateOnMessageDelete(threadID, msgKey); err != nil {
			logger.Error("update_thread_message_indexes_delete_failed", "thread", threadID, "msg", msgID, "error", err)
			return err
		}
	} else {
		// On save, update counts and deltas
		if err := index.UpdateOnMessageSave(threadID, msg.TS, msg.TS); err != nil {
			logger.Error("update_thread_message_indexes_failed", "thread", threadID, "msg", msgID, "error", err)
			return err
		}
	}

	return nil
}
