package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"progressdb/pkg/models"
	"progressdb/pkg/security"
	"progressdb/pkg/utils"

	"github.com/cockroachdb/pebble"
)

var db *pebble.DB

// seq provides a small counter to reduce key collisions when multiple
// messages share the same nanosecond timestamp.
var seq uint64

// Open opens (or creates) a Pebble database at the given path and keeps
// a global handle for simple usage in this package.
func Open(path string) error {
	var err error
	logger.Log.Info("opening_pebble_db", zap.String("path", path))
	db, err = pebble.Open(path, &pebble.Options{})
	if err != nil {
		logger.Log.Error("pebble_open_failed", zap.String("path", path), zap.Error(err))
		return err
	}
	logger.Log.Info("pebble_opened", zap.String("path", path))
	return nil
}

// Close closes the opened pebble DB if present.
func Close() error {
	if db == nil {
		return nil
	}
	if err := db.Close(); err != nil {
		return err
	}
	db = nil
	logger.Log.Info("pebble_closed")
	return nil
}

// Ready reports whether the store is opened and ready.
func Ready() bool {
	return db != nil
}

// SaveMessage appends a message to a thread by inserting a new key with
// a sortable timestamp prefix. Messages are ordered by insertion time.
// SaveMessage appends a message to a thread and indexes it by message ID
// so messages can be looked up and versioned by ID. If msgID is empty,
// only the thread entry is written.
func SaveMessage(threadID, msgID string, msg models.Message) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	// Key format: thread:<threadID>:msg:<unix_nano_padded>-<seq>
	ts := time.Now().UTC().UnixNano()
	s := atomic.AddUint64(&seq, 1)
	key := fmt.Sprintf("thread:%s:msg:%020d-%06d", threadID, ts, s)

	// ensure msgID reflects the message's id when caller omitted it
	if msgID == "" && msg.ID != "" {
		msgID = msg.ID
	}

	// Prepare data for storage (possibly encrypted)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// if encryption is enabled, encrypt the message body or fields according to the policy
	if security.EncryptionEnabled() {
		sthr, terr := GetThread(threadID)
		if terr != nil {
			return fmt.Errorf("encryption enabled but thread metadata missing: %w", terr)
		}
		var th models.Thread
		if err := json.Unmarshal([]byte(sthr), &th); err != nil {
			return fmt.Errorf("invalid thread metadata: %w", err)
		}
		encBody, err := security.EncryptMessageBody(&msg, th)
		if err != nil {
			return fmt.Errorf("encryption failed: %w", err)
		}
		// If EncryptMessageBody returns a new body, update msg.Body and marshal
		if encBody != nil {
			msg.Body = encBody
			nb, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("failed to marshal message after encryption: %w", err)
			}
			data = nb
		}
	}

	// save to database
	if err := db.Set([]byte(key), data, pebble.Sync); err != nil {
		logger.Log.Error("save_message_failed", zap.String("thread", threadID), zap.String("key", key), zap.Error(err))
		return err
	}
	logger.Log.Info("message_saved", zap.String("thread", threadID), zap.String("key", key), zap.String("msg_id", msgID))

	// Also index by message ID for quick lookup of versions.
	if msgID != "" {
		// store version under explicit version namespace
		idxKey := fmt.Sprintf("version:msg:%s:%020d-%06d", msgID, ts, s)
		if err := db.Set([]byte(idxKey), data, pebble.Sync); err != nil {
			logger.Log.Error("save_message_index_failed", zap.String("idxKey", idxKey), zap.Error(err))
			return err
		}
	}
	return nil
}

// ListMessages returns all messages for a thread in insertion order.
func ListMessages(threadID string, limit ...int) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	// build prefix for thread messages
	prefix := []byte("thread:" + threadID + ":msg:")
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	// preload thread key id if encryption is enabled
	var threadKeyID string
	if security.EncryptionEnabled() {
		if s, e := GetThread(threadID); e == nil {
			var th models.Thread
			if json.Unmarshal([]byte(s), &th) == nil {
				threadKeyID = th.KMS.KeyID
			}
		}
	}
	// set max limit if provided
	max := -1
	if len(limit) > 0 {
		max = limit[0]
	}
	// iterate over all messages with the prefix
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		// copy value
		v := append([]byte(nil), iter.Value()...)
		// decrypt if needed
		if security.EncryptionEnabled() {
			logger.Log.Debug("encryption_enabled_listmessages", zap.String("threadID", threadID), zap.String("threadKeyID", threadKeyID))
			if security.EncryptionHasFieldPolicy() {
				if threadKeyID == "" {
					logger.Log.Error("encryption_no_thread_key", zap.String("threadID", threadID))
					return nil, fmt.Errorf("encryption enabled but no thread key available for message")
				}
				var m models.Message
				if err := json.Unmarshal(v, &m); err != nil {
					logger.Log.Error("listmessages_invalid_message_json", zap.ByteString("value", v), zap.Error(err))
					return nil, fmt.Errorf("invalid message JSON: %w", err)
				}
				logger.Log.Debug("listmessages_decrypting_field_policy", zap.String("msgID", m.ID), zap.String("threadKeyID", threadKeyID))
				b, err := security.DecryptMessageBody(&m, threadKeyID)
				if err != nil {
					logger.Log.Error("listmessages_field_decryption_failed", zap.String("msgID", m.ID), zap.String("threadKeyID", threadKeyID), zap.Error(err))
					return nil, fmt.Errorf("field decryption failed: %w", err)
				}
				m.Body = b
				nb, err := json.Marshal(m)
				if err != nil {
					logger.Log.Error("listmessages_marshal_decrypted_failed", zap.String("msgID", m.ID), zap.Error(err))
					return nil, fmt.Errorf("failed to marshal decrypted message: %w", err)
				}
				logger.Log.Debug("listmessages_decrypted_message", zap.String("msgID", m.ID), zap.ByteString("decrypted", nb))
				v = nb
			} else {
				if threadKeyID == "" {
					logger.Log.Error("encryption_no_thread_key", zap.String("threadID", threadID))
					return nil, fmt.Errorf("encryption enabled but no thread key available for message")
				}
				logger.Log.Debug("listmessages_decrypting_full_message", zap.String("threadID", threadID), zap.String("threadKeyID", threadKeyID), zap.ByteString("encrypted", v))
				dec, err := kms.DecryptWithDEK(threadKeyID, v, nil)
				if err != nil {
					logger.Log.Error("listmessages_full_decrypt_failed", zap.String("threadID", threadID), zap.String("threadKeyID", threadKeyID), zap.Error(err), zap.ByteString("encrypted", v))
					return nil, fmt.Errorf("decrypt failed: %w", err)
				}
				logger.Log.Debug("listmessages_decrypted_full_message", zap.String("threadID", threadID), zap.ByteString("decrypted", dec))
				v = dec
			}
		}
		// append to output
		out = append(out, string(v))
		// check limit
		if max > 0 && len(out) >= max {
			break
		}
	}
	// return all messages or error
	return out, iter.Error()
}

// ListMessageVersions returns all stored versions for a given message ID in
// chronological order.
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
				dec, err := kms.DecryptWithDEK(threadKeyID, v, nil)
				if err != nil {
					return nil, fmt.Errorf("decrypt failed: %w", err)
				}
				// Log out the decrypted value for debugging
				logger.Log.Debug("decrypted_message_version", zap.String("threadKeyID", threadKeyID), zap.ByteString("decrypted_value", dec))
				v = dec
			}
		}

		out = append(out, string(v))
	}

	return out, iter.Error()
}

// GetLatestMessage returns the latest version for a message ID or an error
// if none found.
func GetLatestMessage(msgID string) (string, error) {
	vers, err := ListMessageVersions(msgID)
	if err != nil {
		return "", err
	}
	return vers[len(vers)-1], nil
}

// SaveThread stores simple thread metadata under a reserved key.
func SaveThread(threadID, data string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	key := []byte("thread:" + threadID + ":meta")
	if err := db.Set(key, []byte(data), pebble.Sync); err != nil {
		logger.Log.Error("save_thread_failed", zap.String("thread", threadID), zap.Error(err))
		return err
	}
	logger.Log.Info("thread_saved", zap.String("thread", threadID))
	return nil
}

// GetThread returns the stored thread metadata JSON for a given thread ID.
func GetThread(threadID string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("pebble not opened; call store.Open first")
	}
	key := []byte("thread:" + threadID + ":meta")
	v, closer, err := db.Get(key)
	if err != nil {
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}
	return string(v), nil
}

// DeleteThread deletes the thread metadata for a given thread ID.
func DeleteThread(threadID string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	key := []byte("thread:" + threadID + ":meta")
	if err := db.Delete(key, pebble.Sync); err != nil {
		logger.Log.Error("delete_thread_failed", zap.String("thread", threadID), zap.Error(err))
		return err
	}
	logger.Log.Info("thread_deleted", zap.String("thread", threadID))
	return nil
}

// SoftDeleteThread marks the thread as deleted and appends a tombstone message.
func SoftDeleteThread(threadID, actor string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	key := []byte("thread:" + threadID + ":meta")
	v, closer, err := db.Get(key)
	if err != nil {
		logger.Log.Error("soft_delete_load_failed", zap.String("thread", threadID), zap.Error(err))
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	var th models.Thread
	if err := json.Unmarshal(v, &th); err != nil {
		logger.Log.Error("soft_delete_unmarshal_failed", zap.String("thread", threadID), zap.Error(err))
		return err
	}
	th.Deleted = true
	th.DeletedTS = time.Now().UTC().UnixNano()
	nb, _ := json.Marshal(th)
	if err := db.Set(key, nb, pebble.Sync); err != nil {
		logger.Log.Error("soft_delete_save_failed", zap.String("thread", threadID), zap.Error(err))
		return err
	}

	// append tombstone message to the thread
	tomb := models.Message{
		ID:      utils.GenID(),
		Thread:  threadID,
		Author:  actor,
		TS:      time.Now().UTC().UnixNano(),
		Body:    map[string]interface{}{"_event": "thread_deleted", "by": actor},
		Deleted: true,
	}
	// SaveMessage expects a models.Message, not a string
	if err := SaveMessage(threadID, tomb.ID, tomb); err != nil {
		logger.Log.Error("soft_delete_append_tombstone_failed", zap.String("thread", threadID), zap.Error(err))
		return err
	}

	logger.Log.Info("thread_soft_deleted", zap.String("thread", threadID), zap.String("actor", actor))
	return nil
}

// ListThreads returns all saved thread metadata values.
func ListThreads() ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	prefix := []byte("thread:")
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		k := string(iter.Key())
		if strings.HasSuffix(k, ":meta") {
			v := append([]byte(nil), iter.Value()...)
			out = append(out, string(v))
		}
	}
	return out, iter.Error()
}

// ListKeys returns all keys (as strings) that start with the given prefix.
// If prefix is empty it returns all keys in the DB.
func ListKeys(prefix string) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	var pfx []byte
	if prefix != "" {
		pfx = []byte(prefix)
	} else {
		pfx = nil
	}
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	if pfx == nil {
		for iter.First(); iter.Valid(); iter.Next() {
			k := append([]byte(nil), iter.Key()...)
			out = append(out, string(k))
		}
	} else {
		for iter.SeekGE(pfx); iter.Valid(); iter.Next() {
			if !bytes.HasPrefix(iter.Key(), pfx) {
				break
			}
			k := append([]byte(nil), iter.Key()...)
			out = append(out, string(k))
		}
	}
	return out, iter.Error()
}

// GetKey returns the raw value for the given key.
func GetKey(key string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("pebble not opened; call store.Open first")
	}
	v, closer, err := db.Get([]byte(key))
	if err != nil {
		logger.Log.Error("get_key_failed", zap.String("key", key), zap.Error(err))
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}
	logger.Log.Debug("get_key_ok", zap.String("key", key), zap.Int("len", len(v)))
	return string(v), nil
}

// SaveKey stores an arbitrary key/value pair. Use with caution; callers should
// choose a safe namespace (e.g. "kms:dek:").
func SaveKey(key string, value []byte) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	if err := db.Set([]byte(key), value, pebble.Sync); err != nil {
		logger.Log.Error("save_key_failed", zap.String("key", key), zap.Error(err))
		return err
	}
	logger.Log.Debug("save_key_ok", zap.String("key", key), zap.Int("len", len(value)))
	return nil
}

// DBIter returns a raw Pebble iterator for low-level operations. Caller must
// close the iterator when done.
func DBIter() (*pebble.Iterator, error) {
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	return db.NewIter(&pebble.IterOptions{})
}

// DBSet writes a raw key (bytes) into the DB. This is a low-level helper used
// by admin utilities.
func DBSet(key, value []byte) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	return db.Set(key, value, pebble.Sync)
}

// RotateThreadDEK migrates all messages in a thread from the old DEK to a
// new DEK identified by newKeyID. It backs up original values under keys
// prefixed with `backup:migrate:` before overwriting. On success it updates
// the thread->key mapping to the new key.
func RotateThreadDEK(threadID, newKeyID string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	// read existing thread metadata for oldKeyID
	oldKeyID := ""
	if s, err := GetThread(threadID); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err == nil {
			oldKeyID = th.KMS.KeyID
		}
	}
	if oldKeyID == newKeyID {
		return nil
	}
	prefix := []byte("thread:" + threadID + ":msg:")
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return err
	}
	defer iter.Close()

	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		k := append([]byte(nil), iter.Key()...)
		v := append([]byte(nil), iter.Value()...)
		// decrypt with old DEK
		pt, derr := kms.DecryptWithDEK(oldKeyID, v, nil)
		if derr != nil {
			return fmt.Errorf("decrypt message failed: %w", derr)
		}
		// encrypt with new DEK
		ct, _, eerr := kms.EncryptWithDEK(newKeyID, pt, nil)
		// zeroize plaintext
		for i := range pt {
			pt[i] = 0
		}
		if eerr != nil {
			return fmt.Errorf("encrypt with new key failed: %w", eerr)
		}
		// backup original value
		backupKey := append([]byte("backup:migrate:"), k...)
		if err := db.Set(backupKey, v, pebble.Sync); err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}
		// overwrite with new ciphertext
		if err := db.Set(k, ct, pebble.Sync); err != nil {
			return fmt.Errorf("write new ciphertext failed: %w", err)
		}
	}
	// update mapping: persist into thread metadata key so readers use canonical location
	if s, terr := GetThread(threadID); terr == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err == nil {
			th.KMS.KeyID = newKeyID
			if nb, merr := json.Marshal(th); merr == nil {
				if err := SaveThread(th.ID, string(nb)); err != nil {
					return fmt.Errorf("save thread key mapping failed: %w", err)
				}
			}
		}
	}
	return iter.Error()
}

// likelyJSON is a helper function that heuristically determines if a byte slice
// probably contains a JSON object or array. It does this by skipping any leading
// whitespace and checking if the first non-whitespace character is '{' (indicating
// a JSON object) or '[' (indicating a JSON array). This is useful for quickly
// guessing whether a value is JSON-encoded without fully parsing it.
func likelyJSON(b []byte) bool {
	for _, c := range b {
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		return c == '{' || c == '['
	}
	return false
}

// LikelyJSON is an exported version of likelyJSON, allowing other packages to
// check if a byte slice likely contains JSON data.
func LikelyJSON(b []byte) bool { return likelyJSON(b) }
