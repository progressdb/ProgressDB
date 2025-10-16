package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/security"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/utils"

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
		msg.ID = utils.GenID()
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

// returns all versions for a given message in order
func ListMessageVersions(msgID string) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	prefix := []byte("version:msg:" + msgID + ":")
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	var threadKeyID string
	var threadChecked bool
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		v := append([]byte(nil), iter.Value()...)
		if security.EncryptionEnabled() && !threadChecked {
			threadChecked = true
			var msg struct {
				Thread string `json:"thread"`
			}
			if err := json.Unmarshal(v, &msg); err == nil && msg.Thread != "" {
				sthr, terr := GetThread(msg.Thread)
				if terr == nil {
					var th struct {
						KMS struct {
							KeyID string `json:"key_id"`
						} `json:"kms"`
					}
					if json.Unmarshal([]byte(sthr), &th) == nil {
						threadKeyID = th.KMS.KeyID
					}
				}
			} else {
				return nil, fmt.Errorf("cannot determine thread for message version")
			}
		}
		if security.EncryptionEnabled() {
			if security.EncryptionHasFieldPolicy() {
				if threadKeyID == "" {
					return nil, fmt.Errorf("encryption enabled but no thread key available for message version")
				}
				var mm models.Message
				if err := json.Unmarshal(v, &mm); err != nil {
					return nil, fmt.Errorf("invalid message JSON: %w", err)
				}
				decBody, decErr := security.DecryptMessageBody(&mm, threadKeyID)
				if decErr != nil {
					return nil, fmt.Errorf("field decryption failed: %w", decErr)
				}
				mm.Body = decBody
				nb, err := json.Marshal(mm)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal decrypted message: %w", err)
				}
				v = nb
			} else {
				if threadKeyID == "" {
					return nil, fmt.Errorf("encryption enabled but no thread key available for message version")
				}
				var mm models.Message
				if err := json.Unmarshal(v, &mm); err == nil {
					b, derr := security.DecryptMessageBody(&mm, threadKeyID)
					if derr != nil {
						return nil, fmt.Errorf("decrypt failed: %w", derr)
					}
					mm.Body = b
					nb, merr := json.Marshal(mm)
					if merr != nil {
						return nil, fmt.Errorf("failed to marshal decrypted message: %w", merr)
					}
					v = nb
				} else {
					dec, err := kms.DecryptWithDEK(threadKeyID, v, nil)
					if err != nil {
						return nil, fmt.Errorf("decrypt failed: %w", err)
					}
					logger.Debug("decrypted_message_version", "threadKeyID", threadKeyID, "decrypted_len", len(dec))
					v = dec
				}
			}
		}
		out = append(out, string(v))
	}
	return out, iter.Error()
}

// returns latest version for message, error if not found
func GetLatestMessage(msgID string) (string, error) {
	tr := telemetry.Track("store.get_latest_message")
	defer tr.Finish()

	tr.Mark("list_versions")
	vers, err := ListMessageVersions(msgID)
	if err != nil {
		return "", err
	}
	if len(vers) == 0 {
		return "", fmt.Errorf("no versions found for message %s", msgID)
	}
	return vers[len(vers)-1], nil
}

// deletes message and all version keys
func PurgeMessagePermanently(messageID string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	vprefix := []byte("version:msg:" + messageID + ":")
	vi, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return err
	}
	defer vi.Close()
	var keys [][]byte
	for vi.SeekGE(vprefix); vi.Valid(); vi.Next() {
		if !bytes.HasPrefix(vi.Key(), vprefix) {
			break
		}
		keys = append(keys, append([]byte(nil), vi.Key()...))
	}
	for _, k := range keys {
		if err := db.Delete(k, writeOpt(true)); err != nil {
			logger.Error("purge_message_delete_failed", "key", string(k), "error", err)
		}
	}
	logger.Info("purge_message_completed", "msg", messageID, "deleted_keys", len(keys))
	return nil
}
