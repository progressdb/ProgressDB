package store

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"strings"
	"sync"
	"time"

	"progressdb/pkg/models"
	"progressdb/pkg/security"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/utils"

	"github.com/cockroachdb/pebble"
	"sync/atomic"
)

var db *pebble.DB
var dbPath string
var pendingWrites uint64

// seq provides a small counter to reduce key collisions when multiple
// messages share the same nanosecond timestamp.
var seq uint64

var (
	// per-thread locks to avoid concurrent RMW races in a single process
	threadLocks = make(map[string]*sync.Mutex)
	locksMu     sync.Mutex
)

// getThreadLock returns a mutex for a thread, creating it if necessary.
func getThreadLock(threadID string) *sync.Mutex {
	locksMu.Lock()
	defer locksMu.Unlock()
	if l, ok := threadLocks[threadID]; ok {
		return l
	}
	l := &sync.Mutex{}
	threadLocks[threadID] = l
	return l
}

// computeMaxSeq scans existing message keys for a thread and returns the
// highest sequence suffix observed. If no messages exist it returns 0.
func computeMaxSeq(threadID string) (uint64, error) {
	mp, merr := MsgPrefix(threadID)
	if merr != nil {
		return 0, merr
	}
	prefix := []byte(mp)
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return 0, err
	}
	defer iter.Close()
	var max uint64
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		k := string(iter.Key())
		// parse using canonical parser
		_, _, s, perr := ParseMsgKey(k)
		if perr != nil {
			// skip keys that don't parse
			continue
		}
		if s > max {
			max = s
		}
	}
	return max, iter.Error()
}

// MaxSeqForThread is an exported wrapper that returns the highest sequence
// suffix observed for messages in a thread. It is intended for migration
// and admin tooling.
func MaxSeqForThread(threadID string) (uint64, error) {
	return computeMaxSeq(threadID)
}

// Open opens (or creates) a Pebble database at the given path and keeps
// a global handle for simple usage in this package.
func Open(path string) error {
	var err error
	db, err = pebble.Open(path, &pebble.Options{})
	if err != nil {
		logger.Error("pebble_open_failed", "path", path, "error", err)
		return err
	}
	// remember path for best-effort metrics
	dbPath = path
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
	return nil
}

// ApplyBatch applies a prepared pebble.Batch to the DB. If sync is true
// the write will be performed with a sync to disk; otherwise it's applied
// without forcing an fsync (group commit strategy can be used externally).
func ApplyBatch(batch *pebble.Batch, sync bool) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	var err error
	if sync {
		err = db.Apply(batch, pebble.Sync)
	} else {
		err = db.Apply(batch, pebble.NoSync)
	}
	if err != nil {
		logger.Error("pebble_apply_batch_failed", "error", err)
	}
	if err == nil {
		// record that we wrote some data (used by flusher)
		atomic.AddUint64(&pendingWrites, 1)
	}
	return err
}

// RecordWrite increments pending write counter by n (best-effort)
func RecordWrite(n int) {
	if n <= 0 {
		return
	}
	atomic.AddUint64(&pendingWrites, uint64(n))
}

// GetPendingWrites returns an approximate number of pending writes since last sync.
func GetPendingWrites() uint64 {
	return atomic.LoadUint64(&pendingWrites)
}

// ResetPendingWrites resets the pending write counter to zero.
func ResetPendingWrites() {
	atomic.StoreUint64(&pendingWrites, 0)
}

// ForceSync writes a tiny marker entry and forces a WAL fsync via pebble.Sync.
// This is a pragmatic group-commit helper.
func ForceSync() error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	// overwrite a single sync marker key to avoid growth
	key := []byte("__progressdb_wal_sync_marker__")
	val := []byte(time.Now().UTC().Format(time.RFC3339Nano))
	if err := db.Set(key, val, pebble.Sync); err != nil {
		logger.Error("pebble_force_sync_failed", "err", err)
		return err
	}
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
func SaveMessage(ctx context.Context, threadID, msgID string, msg models.Message) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	// Key format: thread:<threadID>:msg:<unix_nano_padded>-<seq>

	// ensure msgID reflects the message's id when caller omitted it
	if msgID == "" && msg.ID != "" {
		msgID = msg.ID
	}
	// If neither caller-provided msgID nor message.ID is present, generate
	// a server-side ID so we always index message versions consistently.
	if msgID == "" && msg.ID == "" {
		msg.ID = utils.GenID()
		msgID = msg.ID
	}

	// Prepare data for storage (possibly encrypted)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// if encryption is enabled, encrypt the message body or fields according to the policy
	if security.EncryptionEnabled() {
		getThreadSpan := telemetry.StartSpanNoCtx("store.get_thread")
		sthr, terr := GetThread(threadID)
		getThreadSpan()
		if terr != nil {
			return fmt.Errorf("encryption enabled but thread metadata missing: %w", terr)
		}
		var th models.Thread
		if err := json.Unmarshal([]byte(sthr), &th); err != nil {
			return fmt.Errorf("invalid thread metadata: %w", err)
		}
		encSpan := telemetry.StartSpanNoCtx("store.encrypt_message")
		encBody, err := security.EncryptMessageBody(&msg, th)
		encSpan()
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

	// Before persisting, obtain and bump the per-thread LastSeq so we can
	// persist a durable per-thread sequence suffix. Guard with an in-process
	// lock so concurrent writers in this process don't collide.
	lock := getThreadLock(threadID)
	lock.Lock()
	defer lock.Unlock()

	// load thread metadata to obtain LastSeq
	var th models.Thread
	sthr2, terr2 := GetThread(threadID)
	if terr2 == nil {
		_ = json.Unmarshal([]byte(sthr2), &th)
	}
	// If LastSeq is zero, attempt to compute a starting value from existing keys
	if th.LastSeq == 0 {
		compSpan := telemetry.StartSpanNoCtx("store.compute_max_seq")
		if max, err := computeMaxSeq(threadID); err == nil {
			th.LastSeq = max
		}
		compSpan()
	}
	// bump per-thread seq
	th.LastSeq = th.LastSeq + 1

	ts := time.Now().UTC().UnixNano()
	s := th.LastSeq
	k, kerr := MsgKey(threadID, ts, s)
	if kerr != nil {
		return fmt.Errorf("failed to build message key: %w", kerr)
	}
	key := k

	// persist updated thread meta and message atomically using a write batch.
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
	batch.Set([]byte(mkey), nb, pebble.Sync)
	batch.Set([]byte(key), data, pebble.Sync)
	if msgID != "" {
		ik, ikerr := VersionKey(msgID, ts, s)
		if ikerr != nil {
			return fmt.Errorf("failed to build version index key: %w", ikerr)
		}
		batch.Set([]byte(ik), data, pebble.Sync)
	}
	dbSpan := telemetry.StartSpanNoCtx("store.db_apply")
	if err := db.Apply(batch, pebble.Sync); err != nil {
		dbSpan()
		logger.Error("save_message_failed", "thread", threadID, "key", key, "error", err)
		return err
	}
	dbSpan()
	logger.Info("message_saved", "thread", threadID, "key", key, "msg_id", msgID)
	return nil
}

// ListMessages returns all messages for a thread in insertion order.
func ListMessages(threadID string, limit ...int) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	// build prefix for thread messages
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
	// preload thread key id if encryption is enabled
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
				// The stored value may be a JSON message where only the body is
				// encrypted (body -> {"_enc":"gcm","v":"<base64>"}). Try to
				// unmarshal and delegate to the canonical decrypt helper. If that
				// fails (legacy/raw ciphertext), fall back to direct KMS decrypt.
				var m models.Message
				if err := json.Unmarshal(v, &m); err == nil {
					b, derr := security.DecryptMessageBody(&m, threadKeyID)
					if derr != nil {
						logger.Error("listmessages_full_decrypt_failed", "threadID", threadID, "threadKeyID", threadKeyID, "error", derr)
						return nil, fmt.Errorf("decrypt failed: %w", derr)
					}
					m.Body = b
					nb, merr := json.Marshal(m)
					if merr != nil {
						logger.Error("listmessages_marshal_decrypted_failed", "msgID", m.ID, "error", merr)
						return nil, fmt.Errorf("failed to marshal decrypted message: %w", merr)
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
	// check if db is open
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}

	// build prefix for message versions
	prefix := []byte("version:msg:" + msgID + ":")

	// create iterator for pebble
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var out []string
	var threadKeyID string
	var threadChecked bool

	// iterate over all keys with the prefix
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		// stop if key does not match prefix
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}

		// copy value bytes
		v := append([]byte(nil), iter.Value()...)

		// if encryption is enabled and we haven't checked thread key yet
		if security.EncryptionEnabled() && !threadChecked {
			threadChecked = true
			// try to get thread id from message json
			var msg struct {
				Thread string `json:"thread"`
			}
			if err := json.Unmarshal(v, &msg); err == nil && msg.Thread != "" {
				// get thread metadata
				sthr, terr := GetThread(msg.Thread)
				if terr == nil {
					// extract key id from thread metadata
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
				// cannot determine thread for this message version
				return nil, fmt.Errorf("cannot determine thread for message version")
			}
		}

		// if encryption is enabled, decrypt the value
		if security.EncryptionEnabled() {
			// field-level encryption policy
			if security.EncryptionHasFieldPolicy() {
				// must have thread key id
				if threadKeyID == "" {
					return nil, fmt.Errorf("encryption enabled but no thread key available for message version")
				}
				// unmarshal message and decrypt body
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
				// full-value encryption
				if threadKeyID == "" {
					return nil, fmt.Errorf("encryption enabled but no thread key available for message version")
				}
				// Attempt to unmarshal the stored value into a Message and
				// delegate to the canonical decrypt helper; this handles the
				// common case where only the body is embedded as
				// {"_enc":"gcm","v":"<base64>"}.
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
					// log decrypted length for debugging
					logger.Debug("decrypted_message_version", "threadKeyID", threadKeyID, "decrypted_len", len(dec))
					v = dec
				}
			}
		}

		// append the (possibly decrypted) value to output
		out = append(out, string(v))
	}

	// return all versions or error from iterator
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
		return "", fmt.Errorf("no versions found for message %s", msgID)
	}
	return vers[len(vers)-1], nil
}

// SaveThread stores simple thread metadata under a reserved key.
func SaveThread(threadID, data string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	if err := db.Set([]byte(tk), []byte(data), pebble.Sync); err != nil {
		logger.Error("save_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_saved", "thread", threadID)
	return nil
}

// GetThread returns the stored thread metadata JSON for a given thread ID.
func GetThread(threadID string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := ThreadMetaKey(threadID)
	if err != nil {
		return "", fmt.Errorf("invalid thread id: %w", err)
	}
	v, closer, err := db.Get([]byte(tk))
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
	tk, err := ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	if err := db.Delete([]byte(tk), pebble.Sync); err != nil {
		logger.Error("delete_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_deleted", "thread", threadID)
	return nil
}

// SoftDeleteThread marks the thread as deleted and appends a tombstone message.
func SoftDeleteThread(threadID, actor string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, terr := ThreadMetaKey(threadID)
	if terr != nil {
		return terr
	}
	key := []byte(tk)
	v, closer, err := db.Get(key)
	if err != nil {
		logger.Error("soft_delete_load_failed", "thread", threadID, "error", err)
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	var th models.Thread
	if err := json.Unmarshal(v, &th); err != nil {
		logger.Error("soft_delete_unmarshal_failed", "thread", threadID, "error", err)
		return err
	}
	th.Deleted = true
	th.DeletedTS = time.Now().UTC().UnixNano()
	nb, _ := json.Marshal(th)
	if err := db.Set(key, nb, pebble.Sync); err != nil {
		logger.Error("soft_delete_save_failed", "thread", threadID, "error", err)
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
	// SaveMessage expects a context, pass background here (internal path)
	if err := SaveMessage(context.Background(), threadID, tomb.ID, tomb); err != nil {
		logger.Error("soft_delete_append_tombstone_failed", "thread", threadID, "error", err)
		return err
	}

	logger.Info("thread_soft_deleted", "thread", threadID, "actor", actor)
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
		// Avoid noisy ERROR-level logs for missing keys; callers can decide
		// how to handle NotFound. Log at Debug level for missing keys so
		// normal absent-key flows (e.g. first-run/version key) are quiet.
		if errors.Is(err, pebble.ErrNotFound) {
			logger.Debug("get_key_missing", "key", key)
		} else {
			logger.Error("get_key_failed", "key", key, "error", err)
		}
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}
	logger.Debug("get_key_ok", "key", key, "len", len(v))
	return string(v), nil
}

// IsNotFound reports whether the provided error originates from Pebble's
// not-found sentinel. Callers can use this to handle missing keys without
// relying on string matching.
func IsNotFound(err error) bool {
	return errors.Is(err, pebble.ErrNotFound)
}

// SaveKey stores an arbitrary key/value pair. Use with caution; callers should
// choose a safe namespace (e.g. "kms:dek:").
func SaveKey(key string, value []byte) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	if err := db.Set([]byte(key), value, pebble.Sync); err != nil {
		logger.Error("save_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("save_key_ok", "key", key, "len", len(value))
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

// DeleteKey removes the given key from the DB.
func DeleteKey(key string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	if err := db.Delete([]byte(key), pebble.Sync); err != nil {
		logger.Error("delete_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("delete_key_ok", "key", key)
	return nil
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
			if th.KMS != nil {
				oldKeyID = th.KMS.KeyID
			}
		}
	}
	if oldKeyID == newKeyID {
		return nil
	}
	mp, merr := MsgPrefix(threadID)
	if merr != nil {
		return merr
	}
	prefix := []byte(mp)
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
		// decrypt with old DEK. The stored value may be either raw
		// ciphertext (legacy) or a JSON-encoded message with an embedded
		// encrypted body. Handle both cases.
		if likelyJSON(v) {
			// try to unmarshal into a message and decrypt the embedded body
			var mm models.Message
			if err := json.Unmarshal(v, &mm); err == nil {
				// decrypt the message body using the canonical helper
				decBody, derr := security.DecryptMessageBody(&mm, oldKeyID)
				if derr != nil {
					return fmt.Errorf("decrypt message failed: %w", derr)
				}
				// marshal the decrypted body to obtain plaintext bytes
				pt, merr := json.Marshal(decBody)
				if merr != nil {
					return fmt.Errorf("marshal plaintext failed: %w", merr)
				}
				// encrypt plaintext with new DEK
				ct, _, eerr := kms.EncryptWithDEK(newKeyID, pt, nil)
				// zeroize plaintext
				for i := range pt {
					pt[i] = 0
				}
				if eerr != nil {
					return fmt.Errorf("encrypt with new key failed: %w", eerr)
				}
				// replace embedded body with new ciphertext (base64)
				mm.Body = map[string]interface{}{"_enc": "gcm", "v": base64.StdEncoding.EncodeToString(ct)}
				nb, merr := json.Marshal(mm)
				if merr != nil {
					return fmt.Errorf("failed to marshal migrated message: %w", merr)
				}
				backupKey := append([]byte("backup:migrate:"), k...)
				if err := db.Set(backupKey, v, pebble.Sync); err != nil {
					return fmt.Errorf("backup failed: %w", err)
				}
				if err := db.Set(k, nb, pebble.Sync); err != nil {
					return fmt.Errorf("write new ciphertext failed: %w", err)
				}
				continue
			}
			// fallthrough to raw-case if unmarshal failed
		}

		// raw ciphertext path (legacy)
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
			if th.KMS == nil {
				th.KMS = &models.KMSMeta{}
			}
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

// PurgeThreadPermanently deletes a thread and all messages/versions under it.
func PurgeThreadPermanently(threadID string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	// prefix for thread keys
	tp, terr := ThreadPrefix(threadID)
	if terr != nil {
		return terr
	}
	prefix := []byte(tp)
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return err
	}
	defer iter.Close()
	// Delete keys in batches while iterating to bound memory usage.
	const deleteBatchSize = 1000
	var batch [][]byte
	deleteBatch := func(keys [][]byte) {
		for _, k := range keys {
			if err := db.Delete(k, pebble.Sync); err != nil {
				logger.Error("purge_delete_failed", "key", string(k), "error", err)
			}
		}
	}

	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		k := append([]byte(nil), iter.Key()...)
		batch = append(batch, k)
		// try to unmarshal message to also delete versions index
		v := append([]byte(nil), iter.Value()...)
		var m models.Message
		if err := json.Unmarshal(v, &m); err == nil && m.ID != "" {
			// delete version index keys for this message id
			vprefix := []byte("version:msg:" + m.ID + ":")
			vi, _ := db.NewIter(&pebble.IterOptions{})
			if vi != nil {
				for vi.SeekGE(vprefix); vi.Valid(); vi.Next() {
					if !bytes.HasPrefix(vi.Key(), vprefix) {
						break
					}
					kk := append([]byte(nil), vi.Key()...)
					batch = append(batch, kk)
					if len(batch) >= deleteBatchSize {
						deleteBatch(batch)
						batch = batch[:0]
					}
				}
				vi.Close()
			}
		}
		if len(batch) >= deleteBatchSize {
			deleteBatch(batch)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		deleteBatch(batch)
	}
	// delete thread meta explicitly
	_ = DeleteThread(threadID)
	// deleted_keys unknown with streaming deletes
	logger.Info("purge_thread_completed", "thread", threadID, "deleted_keys", 0)
	return nil
}

// PurgeMessagePermanently deletes a single message and its version indexes.
func PurgeMessagePermanently(messageID string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	// delete version keys
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
		if err := db.Delete(k, pebble.Sync); err != nil {
			logger.Error("purge_message_delete_failed", "key", string(k), "error", err)
		}
	}
	logger.Info("purge_message_completed", "msg", messageID, "deleted_keys", len(keys))
	return nil
}
