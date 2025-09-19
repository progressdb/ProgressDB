package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"progressdb/pkg/logger"
	"strings"
	"sync"
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
var threadLocks sync.Map // map[string]*sync.Mutex

func lockForThread(threadID string) func() {
	v, _ := threadLocks.LoadOrStore(threadID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return func() { mu.Unlock() }
}

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
func SaveMessage(threadID, msgID, msg string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	// Key format: thread:<threadID>:msg:<unix_nano_padded>-<seq>
	ts := time.Now().UTC().UnixNano()
	s := atomic.AddUint64(&seq, 1)
	key := fmt.Sprintf("thread:%s:msg:%020d-%06d", threadID, ts, s)
	data := []byte(msg)

	// Encryption logic: check if encryption is enabled, then check if field policy is enabled, then proceed accordingly.
	if security.EncryptionEnabled() {
		if security.HasFieldPolicy() {
			// Field-policy encryption is enabled and encryption is on.
			out, err := security.EncryptJSONFields(data)
			if err != nil {
				return fmt.Errorf("field encryption failed: %w", err)
			}
			data = out
		} else {
			// No field policy: perform full-message DEK encryption using the thread's configured DEK.
			sthr, terr := GetThread(threadID)
			if terr != nil {
				return fmt.Errorf("encryption enabled but thread metadata missing: %w", terr)
			}
			var th models.Thread
			if err := json.Unmarshal([]byte(sthr), &th); err != nil {
				return fmt.Errorf("invalid thread metadata: %w", err)
			}
			keyID := th.KMS.KeyID
			if keyID == "" {
				return fmt.Errorf("encryption enabled but no DEK configured for thread %s", threadID)
			}
			enc, _, eerr := security.EncryptWithDEK(keyID, data, nil)
			if eerr != nil {
				return eerr
			}
			data = enc
		}
	} else {
		// If encryption is not enabled but a field policy is configured, this is a configuration error.
		if security.HasFieldPolicy() {
			logger.Log.Error("field encryption policy configured but encryption not enabled")
		}
		// Otherwise, store plaintext.
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
func ListMessages(threadID string) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	prefix := []byte("thread:" + threadID + ":msg:")
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var out []string
	// preload thread's KeyID once for efficient per-message decrypt
	var threadKeyID string
	if security.EncryptionEnabled() {
		if sthr, terr := GetThread(threadID); terr == nil {
			var th models.Thread
			if err := json.Unmarshal([]byte(sthr), &th); err == nil {
				threadKeyID = th.KMS.KeyID
			}
		}
	}

	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		// Capture the value before advancing.
		v := append([]byte(nil), iter.Value()...)
		if security.EncryptionEnabled() {
			if security.HasFieldPolicy() {
				// Try full-message decrypt first; if it fails, attempt field-level decrypt.
				if dec, err := security.Decrypt(v); err == nil {
					v = dec
				} else {
					if outJSON, err := security.DecryptJSONFields(v); err == nil {
						v = outJSON
					} else if likelyJSON(v) {
						// tolerate legacy plaintext JSON
					} else {
						return nil, err
					}
				}
			} else {
				// Prefer provider-backed per-thread DEK decryption when available.
				if threadKeyID != "" {
					if dec, derr := security.DecryptWithDEK(threadKeyID, v, nil); derr == nil {
						v = dec
					} else {
						// fall back to generic decrypt which may handle embedded master-key mode
						if dec2, err := security.Decrypt(v); err == nil {
							v = dec2
						} else if !likelyJSON(v) {
							return nil, derr
						}
					}
				} else {
					if dec, err := security.Decrypt(v); err != nil {
						// tolerate legacy plaintext JSON values
						if !likelyJSON(v) {
							return nil, err
						}
					} else {
						v = dec
					}
				}
			}
		}
		out = append(out, string(v))
	}
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
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		v := append([]byte(nil), iter.Value()...)
		if security.EncryptionEnabled() {
			// Reuse the same decrypt logic as ListMessages
			if security.HasFieldPolicy() {
				if dec, err := security.Decrypt(v); err == nil {
					v = dec
				} else if outJSON, err := security.DecryptJSONFields(v); err == nil {
					v = outJSON
				} else if likelyJSON(v) {
					// tolerate legacy plaintext JSON
				} else {
					return nil, err
				}
			} else {
				// Prefer provider-backed per-thread DEK decryption when available.
				if dec, err := security.Decrypt(v); err != nil {
					if !likelyJSON(v) {
						return nil, err
					}
				} else {
					v = dec
				}
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
	if len(vers) == 0 {
		// Fallback for legacy records: scan all threads and their messages
		// to find any stored message with this id. This is expensive and
		// intended only as a compatibility fallback for older records that
		// were stored without a msgid index.
		logger.Log.Info("get_latest_message_fallback", zap.String("msgid", msgID))
		threads, terr := ListThreads()
		if terr != nil {
			return "", fmt.Errorf("message not found")
		}
		var found []string
		for _, t := range threads {
			// t is JSON bytes of thread metadata (string). Try to extract id
			// without depending on models package.
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(t), &m); err != nil {
				continue
			}
			tid, _ := m["id"].(string)
			if tid == "" {
				continue
			}
			msgs, merr := ListMessages(tid)
			if merr != nil {
				continue
			}
			for _, s := range msgs {
				// try to unmarshal message and check id
				var mm map[string]interface{}
				if err := json.Unmarshal([]byte(s), &mm); err != nil {
					continue
				}
				if idv, ok := mm["id"].(string); ok && idv == msgID {
					found = append(found, s)
				}
			}
		}
		if len(found) == 0 {
			return "", fmt.Errorf("message not found")
		}
		logger.Log.Info("get_latest_message_found_via_scan", zap.String("msgid", msgID))
		return found[len(found)-1], nil
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
	tb, _ := json.Marshal(tomb)
	if err := SaveMessage(threadID, tomb.ID, string(tb)); err != nil {
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
	// collect only keys that end with :meta under the thread: prefix
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
			// copy key
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

// SaveThreadKey maps a threadID to a keyID for its DEK.
// Deprecated: per-thread DEK mapping is now stored in thread metadata (thread:<id>:meta).
// Use GetThread to read the canonical KMS metadata at thread.KMS.KeyID.

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
		pt, derr := security.DecryptWithDEK(oldKeyID, v, nil)
		if derr != nil {
			return fmt.Errorf("decrypt message failed: %w", derr)
		}
		// encrypt with new DEK
		ct, _, eerr := security.EncryptWithDEK(newKeyID, pt, nil)
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

func likelyJSON(b []byte) bool {
	// Trim leading spaces and check first byte
	for _, c := range b {
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		return c == '{' || c == '['
	}
	return false
}

// LikelyJSON is an exported wrapper for internal likelyJSON helper.
func LikelyJSON(b []byte) bool { return likelyJSON(b) }
