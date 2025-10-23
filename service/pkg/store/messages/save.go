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

// saves message with sequencing (encryption done in compute phase)
func SaveMessage(ctx context.Context, threadID, msgID string, encryptedData []byte, ts int64, author string, isDelete bool) error {
	tr := telemetry.Track("storedb.save_message")
	defer tr.Finish()

	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}

	// read thread for decryption
	threadKey := keys.GenThreadKey(threadID)
	threadData, closer, err := storedb.Client.Get([]byte(threadKey))
	if err != nil {
		return fmt.Errorf("failed to get thread: %w", err)
	}
	if closer != nil {
		defer closer.Close()
	}
	var thread models.Thread
	if err := json.Unmarshal(threadData, &thread); err != nil {
		return fmt.Errorf("failed to unmarshal thread: %w", err)
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
	msgKey := keys.GenMessageKey(threadID, msgID, seq)
	versionKey := keys.GenVersionKey(msgID, ts, seq)

	// data is already encrypted
	data := encryptedData

	// move old message to versioning if exists
	if oldData, closer, err := storedb.Client.Get([]byte(msgKey)); err == nil {
		if closer != nil {
			defer closer.Close()
		}
		oldDataCopy := append([]byte(nil), oldData...)
		// decrypt old data to get old TS
		oldDecrypted, err := encryption.DecryptMessageData(thread.KMS, oldDataCopy)
		if err != nil {
			return fmt.Errorf("failed to decrypt old message: %w", err)
		}
		var oldMsg models.Message
		if err := json.Unmarshal(oldDecrypted, &oldMsg); err != nil {
			return fmt.Errorf("failed to unmarshal old message: %w", err)
		}
		oldSeq := seq - 1
		oldVersionKey := keys.GenVersionKey(msgID, oldMsg.TS, oldSeq)
		if err := index.SaveKey(oldVersionKey, oldDataCopy); err != nil {
			logger.Error("save_old_version_failed", "key", oldVersionKey, "error", err)
			return err
		}
		// update version indexes for old
		if err := index.UpdateVersionIndexes(threadID, msgID, oldMsg.TS, oldSeq, oldMsg.TS, oldMsg.TS); err != nil {
			logger.Error("update_version_indexes_old_failed", "thread", threadID, "msg", msgID, "error", err)
			return err
		}
	} else if !storedb.IsNotFound(err) {
		return fmt.Errorf("failed to check existing message: %w", err)
	}

	// save to batch (main DB for current)
	batch := storedb.Client.NewBatch()
	defer batch.Close()
	batch.Set([]byte(msgKey), data, storedb.WriteOpt(true))

	// save version to index DB
	if err := index.SaveKey(versionKey, data); err != nil {
		logger.Error("save_version_failed", "key", versionKey, "error", err)
		return err
	}

	// update version indexes for new
	if err := index.UpdateVersionIndexes(threadID, msgID, ts, seq, ts, ts); err != nil {
		logger.Error("update_version_indexes_failed", "thread", threadID, "msg", msgID, "error", err)
		return err
	}

	tr.Mark("apply")
	if err := storedb.Client.Apply(batch, storedb.WriteOpt(true)); err != nil {
		logger.Error("save_message_failed", "thread", threadID, "msg", msgID, "error", err)
		return err
	}

	logger.Info("message_saved", "thread", threadID, "msg", msgID, "seq", seq)

	// Update thread message indexes
	if isDelete {
		// On delete, add to skips
		if err := index.UpdateOnMessageDelete(threadID, msgKey); err != nil {
			logger.Error("update_thread_message_indexes_delete_failed", "thread", threadID, "msg", msgID, "error", err)
			return err
		}
		// Add to user's deleted messages
		if err := index.UpdateDeletedMessages(author, msgID, true); err != nil {
			logger.Error("update_deleted_messages_failed", "user", author, "msg", msgID, "error", err)
			return err
		}
	} else {
		// On save, update counts and deltas
		if err := index.UpdateOnMessageSave(threadID, ts, ts); err != nil {
			logger.Error("update_thread_message_indexes_failed", "thread", threadID, "msg", msgID, "error", err)
			return err
		}
	}

	return nil
}
