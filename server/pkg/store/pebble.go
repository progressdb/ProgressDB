package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"progressdb/pkg/security"

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
	slog.Info("opening_pebble_db", "path", path)
	db, err = pebble.Open(path, &pebble.Options{})
	if err != nil {
		slog.Error("pebble_open_failed", "path", path, "error", err)
		return err
	}
	slog.Info("pebble_opened", "path", path)
	return nil
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
	// Key format: thread:<threadID>:<unix_nano_padded>-<seq>
	ts := time.Now().UTC().UnixNano()
	s := atomic.AddUint64(&seq, 1)
	key := fmt.Sprintf("thread:%s:%020d-%06d", threadID, ts, s)
	data := []byte(msg)
	if security.Enabled() {
		if security.HasFieldPolicy() {
			if out, err := security.EncryptJSONFields(data); err == nil {
				data = out
			} else {
				// Fallback: full-message encryption if not JSON
				enc, err := security.Encrypt(data)
				if err != nil {
					return err
				}
				data = enc
			}
		} else {
			enc, err := security.Encrypt(data)
			if err != nil {
				return err
			}
			data = enc
		}
	}
	if err := db.Set([]byte(key), data, pebble.Sync); err != nil {
		slog.Error("save_message_failed", "thread", threadID, "key", key, "error", err)
		return err
	}
	slog.Info("message_saved", "thread", threadID, "key", key, "msg_id", msgID)
	// Also index by message ID for quick lookup of versions.
	if msgID != "" {
		idxKey := fmt.Sprintf("msgid:%s:%020d-%06d", msgID, ts, s)
		if err := db.Set([]byte(idxKey), data, pebble.Sync); err != nil {
			slog.Error("save_message_index_failed", "idxKey", idxKey, "error", err)
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
	prefix := []byte("thread:" + threadID + ":")
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
		// Capture the value before advancing.
		v := append([]byte(nil), iter.Value()...)
		if security.Enabled() {
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
				dec, err := security.Decrypt(v)
				if err != nil {
					// tolerate legacy plaintext JSON values
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

// ListMessageVersions returns all stored versions for a given message ID in
// chronological order.
func ListMessageVersions(msgID string) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	prefix := []byte("msgid:" + msgID + ":")
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
		if security.Enabled() {
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
				dec, err := security.Decrypt(v)
				if err != nil {
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
		slog.Info("get_latest_message_fallback", "msgid", msgID)
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
		slog.Info("get_latest_message_found_via_scan", "msgid", msgID)
		return found[len(found)-1], nil
	}
	return vers[len(vers)-1], nil
}

// SaveThread stores simple thread metadata under a reserved key.
func SaveThread(threadID, data string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	key := []byte("threadmeta:" + threadID)
	if err := db.Set(key, []byte(data), pebble.Sync); err != nil {
		slog.Error("save_thread_failed", "thread", threadID, "error", err)
		return err
	}
	slog.Info("thread_saved", "thread", threadID)
	return nil
}

// GetThread returns the stored thread metadata JSON for a given thread ID.
func GetThread(threadID string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("pebble not opened; call store.Open first")
	}
	key := []byte("threadmeta:" + threadID)
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
	key := []byte("threadmeta:" + threadID)
	if err := db.Delete(key, pebble.Sync); err != nil {
		slog.Error("delete_thread_failed", "thread", threadID, "error", err)
		return err
	}
	slog.Info("thread_deleted", "thread", threadID)
	return nil
}

// ListThreads returns all saved thread metadata values.
func ListThreads() ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	prefix := []byte("threadmeta:")
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
		out = append(out, string(v))
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
		slog.Error("get_key_failed", "key", key, "error", err)
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}
	slog.Debug("get_key_ok", "key", key, "len", len(v))
	return string(v), nil
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
