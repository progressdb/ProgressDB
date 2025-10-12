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

	"sync/atomic"

	"github.com/cockroachdb/pebble"
)

var db *pebble.DB
var dbPath string
var pendingWrites uint64
var walDisabled bool

// small counter to avoid key collisions on nanosecond timestamp
var seq uint64

var (
	threadLocks = make(map[string]*sync.Mutex)
	locksMu     sync.Mutex
)

// returns mutex for given thread (creates if needed)
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

// scans messages for thread and returns largest sequence number
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
		_, _, s, perr := ParseMsgKey(k)
		if perr != nil {
			continue
		}
		if s > max {
			max = s
		}
	}
	return max, iter.Error()
}

// wrapper for computeMaxSeq (for migrations/admin)
func MaxSeqForThread(threadID string) (uint64, error) {
	return computeMaxSeq(threadID)
}

// opens/creates pebble DB with WAL settings
func Open(path string, disablePebbleWAL bool, appWALEnabled bool) error {
	var err error
	opts := &pebble.Options{
		DisableWAL: disablePebbleWAL,
	}
	walDisabled = opts.DisableWAL

	if walDisabled && !appWALEnabled {
		logger.Warn("durability_disabled", "durability", "no WAL enabled")
	}

	db, err = pebble.Open(path, opts)
	if err != nil {
		logger.Error("pebble_open_failed", "path", path, "error", err)
		return err
	}
	dbPath = path
	return nil
}

// closes opened pebble DB
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

// applies batch; sync forces fsync if true, else async write
func ApplyBatch(batch *pebble.Batch, sync bool) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	var err error
	err = db.Apply(batch, writeOpt(sync))
	if err != nil {
		logger.Error("pebble_apply_batch_failed", "error", err)
	}
	if err == nil {
		atomic.AddUint64(&pendingWrites, 1)
	}
	return err
}

// increments pending write counter by n
func RecordWrite(n int) {
	if n <= 0 {
		return
	}
	atomic.AddUint64(&pendingWrites, uint64(n))
}

// returns count of pending writes since last sync
func GetPendingWrites() uint64 {
	return atomic.LoadUint64(&pendingWrites)
}

// resets pending write counter
func ResetPendingWrites() {
	atomic.StoreUint64(&pendingWrites, 0)
}

// writes marker key, forces WAL fsync unless disabled (group-commit)
func ForceSync() error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	if walDisabled {
		logger.Debug("pebble_force_sync_noop_wal_disabled")
		return nil
	}
	key := []byte("__progressdb_wal_sync_marker__")
	val := []byte(time.Now().UTC().Format(time.RFC3339Nano))
	if err := db.Set(key, val, writeOpt(true)); err != nil {
		logger.Error("pebble_force_sync_failed", "err", err)
		return err
	}
	return nil
}

// chooses sync/no-sync WriteOptions, always disables sync if WAL disabled
func writeOpt(requestSync bool) *pebble.WriteOptions {
	if requestSync && !walDisabled {
		return pebble.Sync
	}
	return pebble.NoSync
}

// returns true if DB is opened
func Ready() bool {
	return db != nil
}

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

	// handle encryption if enabled
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
		compSpan := telemetry.StartSpanNoCtx("store.compute_max_seq")
		if max, err := computeMaxSeq(threadID); err == nil {
			th.LastSeq = max
		}
		compSpan()
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
	dbSpan := telemetry.StartSpanNoCtx("store.db_apply")
	if err := db.Apply(batch, writeOpt(true)); err != nil {
		dbSpan()
		logger.Error("save_message_failed", "thread", threadID, "key", key, "error", err)
		return err
	}
	dbSpan()
	logger.Info("message_saved", "thread", threadID, "key", key, "msg_id", msgID)
	return nil
}

// lists all messages for thread, ordered by insertion
func ListMessages(threadID string, limit ...int) ([]string, error) {
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
	vers, err := ListMessageVersions(msgID)
	if err != nil {
		return "", err
	}
	if len(vers) == 0 {
		return "", fmt.Errorf("no versions found for message %s", msgID)
	}
	return vers[len(vers)-1], nil
}

// saves thread metadata as JSON
func SaveThread(threadID, data string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	if err := db.Set([]byte(tk), []byte(data), writeOpt(true)); err != nil {
		logger.Error("save_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_saved", "thread", threadID)
	return nil
}

// gets thread metadata JSON for id
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

// deletes thread metadata
func DeleteThread(threadID string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := ThreadMetaKey(threadID)
	if err != nil {
		return fmt.Errorf("invalid thread id: %w", err)
	}
	if err := db.Delete([]byte(tk), writeOpt(true)); err != nil {
		logger.Error("delete_thread_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_deleted", "thread", threadID)
	return nil
}

// marks thread as deleted and adds a tombstone message
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
	if err := db.Set(key, nb, writeOpt(true)); err != nil {
		logger.Error("soft_delete_save_failed", "thread", threadID, "error", err)
		return err
	}
	tomb := models.Message{
		ID:      utils.GenID(),
		Thread:  threadID,
		Author:  actor,
		TS:      time.Now().UTC().UnixNano(),
		Body:    map[string]interface{}{"_event": "thread_deleted", "by": actor},
		Deleted: true,
	}
	// use background context
	if err := SaveMessage(context.Background(), threadID, tomb.ID, tomb); err != nil {
		logger.Error("soft_delete_append_tombstone_failed", "thread", threadID, "error", err)
		return err
	}
	logger.Info("thread_soft_deleted", "thread", threadID, "actor", actor)
	return nil
}

// lists all saved thread metadata as JSON
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

// lists all keys as strings for prefix; returns all if prefix empty
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

// returns raw value for key as string
func GetKey(key string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("pebble not opened; call store.Open first")
	}
	v, closer, err := db.Get([]byte(key))
	if err != nil {
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

// returns true if error is pebble.ErrNotFound
func IsNotFound(err error) bool {
	return errors.Is(err, pebble.ErrNotFound)
}

// stores arbitrary key/value (namespace caution: e.g. "kms:dek:")
func SaveKey(key string, value []byte) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	if err := db.Set([]byte(key), value, writeOpt(true)); err != nil {
		logger.Error("save_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("save_key_ok", "key", key, "len", len(value))
	return nil
}

// returns iterator, caller must close
func DBIter() (*pebble.Iterator, error) {
	if db == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	return db.NewIter(&pebble.IterOptions{})
}

// writes key (bytes) as is, for admin use
func DBSet(key, value []byte) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	return db.Set(key, value, writeOpt(true))
}

// removes key
func DeleteKey(key string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	if err := db.Delete([]byte(key), writeOpt(true)); err != nil {
		logger.Error("delete_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("delete_key_ok", "key", key)
	return nil
}

// migrates all thread messages to new DEK; backs up old data before overwriting
func RotateThreadDEK(threadID, newKeyID string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
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
		if likelyJSON(v) {
			var mm models.Message
			if err := json.Unmarshal(v, &mm); err == nil {
				decBody, derr := security.DecryptMessageBody(&mm, oldKeyID)
				if derr != nil {
					return fmt.Errorf("decrypt message failed: %w", derr)
				}
				pt, merr := json.Marshal(decBody)
				if merr != nil {
					return fmt.Errorf("marshal plaintext failed: %w", merr)
				}
				ct, _, eerr := kms.EncryptWithDEK(newKeyID, pt, nil)
				for i := range pt {
					pt[i] = 0
				}
				if eerr != nil {
					return fmt.Errorf("encrypt with new key failed: %w", eerr)
				}
				mm.Body = map[string]interface{}{"_enc": "gcm", "v": base64.StdEncoding.EncodeToString(ct)}
				nb, merr := json.Marshal(mm)
				if merr != nil {
					return fmt.Errorf("failed to marshal migrated message: %w", merr)
				}
				backupKey := append([]byte("backup:migrate:"), k...)
				if err := db.Set(backupKey, v, writeOpt(true)); err != nil {
					return fmt.Errorf("backup failed: %w", err)
				}
				if err := db.Set(k, nb, writeOpt(true)); err != nil {
					return fmt.Errorf("write new ciphertext failed: %w", err)
				}
				continue
			}
		}
		pt, derr := kms.DecryptWithDEK(oldKeyID, v, nil)
		if derr != nil {
			return fmt.Errorf("decrypt message failed: %w", derr)
		}
		ct, _, eerr := kms.EncryptWithDEK(newKeyID, pt, nil)
		for i := range pt {
			pt[i] = 0
		}
		if eerr != nil {
			return fmt.Errorf("encrypt with new key failed: %w", eerr)
		}
		backupKey := append([]byte("backup:migrate:"), k...)
		if err := db.Set(backupKey, v, writeOpt(true)); err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}
		if err := db.Set(k, ct, writeOpt(true)); err != nil {
			return fmt.Errorf("write new ciphertext failed: %w", err)
		}
	}
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

// true if likely contains JSON object/array based on first non-whitespace
func likelyJSON(b []byte) bool {
	for _, c := range b {
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		return c == '{' || c == '['
	}
	return false
}

// exported version of likelyJSON
func LikelyJSON(b []byte) bool { return likelyJSON(b) }

// deletes thread and all messages/versions; removes in batches
func PurgeThreadPermanently(threadID string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
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
	const deleteBatchSize = 1000
	var batch [][]byte
	deleteBatch := func(keys [][]byte) {
		for _, k := range keys {
			if err := db.Delete(k, writeOpt(true)); err != nil {
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
		v := append([]byte(nil), iter.Value()...)
		var m models.Message
		if err := json.Unmarshal(v, &m); err == nil && m.ID != "" {
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
	_ = DeleteThread(threadID)
	logger.Info("purge_thread_completed", "thread", threadID, "deleted_keys", 0)
	return nil
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
