package store

import (
	"bytes"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"progressdb/pkg/logger"

	"github.com/cockroachdb/pebble"
)

var db *pebble.DB
var dbPath string
var walDisabled bool

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

// returns true if error is pebble.ErrNotFound
func IsNotFound(err error) bool {
	return errors.Is(err, pebble.ErrNotFound)
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
