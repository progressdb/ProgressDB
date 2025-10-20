package store

import (
	"bytes"
	"encoding/json"
	"fmt"

	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/security"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

// lists all messages for thread, ordered by insertion
func ListMessages(threadID string, limit ...int) ([]string, error) {
	tr := telemetry.Track("store.list_messages")
	defer tr.Finish()

	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	mp, merr := MsgPrefix(threadID)
	if merr != nil {
		return nil, merr
	}
	prefix := []byte(mp)
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	var threadKeyID string
	if security.EncryptionEnabled() {
		if s, e := GetThread(threadID); e == nil {
			var th models.Thread
			if json.Unmarshal([]byte(s), &th) == nil {
				if th.KMS != nil {
					threadKeyID = th.KMS.KeyID
				}
			}
		}
	}
	max := -1
	if len(limit) > 0 {
		max = limit[0]
	}
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		v := append([]byte(nil), iter.Value()...)
		if security.EncryptionEnabled() {
			logger.Debug("encryption_enabled_listmessages", "threadID", threadID, "threadKeyID", threadKeyID)
			if security.EncryptionHasFieldPolicy() {
				if threadKeyID == "" {
					logger.Error("encryption_no_thread_key", "threadID", threadID)
					return nil, fmt.Errorf("encryption enabled but no thread key available for message")
				}
				var m models.Message
				if err := json.Unmarshal(v, &m); err != nil {
					logger.Error("listmessages_invalid_message_json", "value", v, "error", err)
					return nil, fmt.Errorf("invalid message JSON: %w", err)
				}
				logger.Debug("listmessages_decrypting_field_policy", "msgID", m.ID, "threadKeyID", threadKeyID)
				b, err := security.DecryptMessageBody(&m, threadKeyID)
				if err != nil {
					logger.Error("listmessages_field_decryption_failed", "msgID", m.ID, "threadKeyID", threadKeyID, "error", err)
					return nil, fmt.Errorf("field decryption failed: %w", err)
				}
				m.Body = b
				nb, err := json.Marshal(m)
				if err != nil {
					logger.Error("listmessages_marshal_decrypted_failed", "msgID", m.ID, "error", err)
					return nil, fmt.Errorf("failed to marshal decrypted message: %w", err)
				}
				logger.Debug("listmessages_decrypted_message", "msgID", m.ID, "decrypted", nb)
				v = nb
			} else {
				if threadKeyID == "" {
					logger.Error("encryption_no_thread_key", "threadID", threadID)
					return nil, fmt.Errorf("encryption enabled but no thread key available for message")
				}
				var m models.Message
				if err := json.Unmarshal(v, &m); err == nil {
					b, derr := security.DecryptMessageBody(&m, threadKeyID)
					if derr != nil {
						logger.Error("listmessages_full_decrypt_failed", "threadID", threadID, "threadKeyID", threadKeyID, "error", derr)
						return nil, fmt.Errorf("decrypt failed: %w", derr)
					}
					m.Body = b
					nb, err := json.Marshal(m)
					if err != nil {
						logger.Error("listmessages_marshal_decrypted_failed", "msgID", m.ID, "error", err)
						return nil, fmt.Errorf("failed to marshal decrypted message: %w", err)
					}
					logger.Debug("listmessages_decrypted_full_message", "threadID", threadID, "decrypted", nb)
					v = nb
				} else {
					logger.Debug("listmessages_decrypting_full_message", "threadID", threadID, "threadKeyID", threadKeyID, "encrypted_len", len(v))
					dec, err := kms.DecryptWithDEK(threadKeyID, v, nil)
					if err != nil {
						logger.Error("listmessages_full_decrypt_failed", "threadID", threadID, "threadKeyID", threadKeyID, "error", err)
						return nil, fmt.Errorf("decrypt failed: %w", err)
					}
					logger.Debug("listmessages_decrypted_full_message", "threadID", threadID, "decrypted_len", len(dec))
					v = dec
				}
			}
		}
		out = append(out, string(v))
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out, iter.Error()
}
