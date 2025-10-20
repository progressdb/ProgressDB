package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/security"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

// saves message; inserts new key for thread, indexes by ID; assigns ID if missing
func SaveMessage(ctx context.Context, threadID, msgID string, msg models.Message) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}

	if msgID == "" && msg.ID != "" {
		msgID = msg.ID
	}
	if msgID == "" && msg.ID == "" {
		msg.ID = GenMessageID()
		msgID = msg.ID
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	tr := telemetry.Track("store.save_message")
	defer tr.Finish()

	// handle encryption if enabled
	if security.EncryptionEnabled() {
		tr.Mark("get_thread")
		sthr, terr := GetThread(threadID)
		if terr != nil {
			return fmt.Errorf("encryption enabled but thread metadata missing: %w", terr)
		}
		var th models.Thread
		if err := json.Unmarshal([]byte(sthr), &th); err != nil {
			return fmt.Errorf("invalid thread metadata: %w", err)
		}
		tr.Mark("encrypt_message")
		encBody, err := security.EncryptMessageBody(&msg, th)
		if err != nil {
			return fmt.Errorf("encryption failed: %w", err)
		}
		if encBody != nil {
			msg.Body = encBody
			nb, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("failed to marshal message after encryption: %w", err)
			}
			data = nb
		}
	}

	// update thread sequence, lock for this thread
	lock := getThreadLock(threadID)
	lock.Lock()
	defer lock.Unlock()

	var th models.Thread
	sthr2, terr2 := GetThread(threadID)
	if terr2 == nil {
		_ = json.Unmarshal([]byte(sthr2), &th)
	}
	if th.LastSeq == 0 {
		tr.Mark("compute_max_seq")
		if max, err := computeMaxSeq(threadID); err == nil {
			th.LastSeq = max
		}
	}
	th.LastSeq++

	ts := time.Now().UTC().UnixNano()
	s := th.LastSeq
	k, kerr := MsgKey(threadID, ts, s)
	if kerr != nil {
		return fmt.Errorf("failed to build message key: %w", kerr)
	}
	key := k

	// save updated thread meta and message atomically in batch
	th.UpdatedTS = time.Now().UTC().UnixNano()
	nb, err := json.Marshal(th)
	if err != nil {
		return fmt.Errorf("failed to marshal thread meta: %w", err)
	}

	batch := new(pebble.Batch)
	mkey, mkerr := ThreadMetaKey(threadID)
	if mkerr != nil {
		return fmt.Errorf("invalid thread id for meta key: %w", mkerr)
	}
	batch.Set([]byte(mkey), nb, writeOpt(true))
	batch.Set([]byte(key), data, writeOpt(true))
	if msgID != "" {
		ik, ikerr := VersionKey(msgID, ts, s)
		if ikerr != nil {
			return fmt.Errorf("failed to build version index key: %w", ikerr)
		}
		batch.Set([]byte(ik), data, writeOpt(true))
	}
	tr.Mark("db_apply")
	if err := db.Apply(batch, writeOpt(true)); err != nil {
		logger.Error("save_message_failed", "thread", threadID, "key", key, "error", err)
		return err
	}
	logger.Info("message_saved", "thread", threadID, "key", key, "msg_id", msgID)
	return nil
}
