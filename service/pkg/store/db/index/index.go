package index

import (
	"bytes"
	"errors"
	"fmt"

	"progressdb/pkg/logger"

	"github.com/cockroachdb/pebble"
)

var IndexDB *pebble.DB
var IndexDBPath string
var IndexWALDisabled bool
var IndexPendingWrites uint64

// opens/creates pebble DB with WAL settings for index storage
func Open(path string, disablePebbleWAL bool, appWALEnabled bool) error {
	var err error
	opts := &pebble.Options{
		DisableWAL: disablePebbleWAL,
	}
	IndexWALDisabled = opts.DisableWAL

	if IndexWALDisabled && !appWALEnabled {
		logger.Warn("durability_disabled", "durability", "no WAL enabled for index DB")
	}

	IndexDB, err = pebble.Open(path, opts)
	if err != nil {
		logger.Error("pebble_open_failed", "path", path, "error", err)
		return err
	}
	IndexDBPath = path
	return nil
}

// closes opened pebble DB
func Close() error {
	if IndexDB == nil {
		return nil
	}
	if err := IndexDB.Close(); err != nil {
		return err
	}
	IndexDB = nil
	return nil
}

// returns true if DB is opened
func Ready() bool {
	return IndexDB != nil
}

// returns true if error is pebble.ErrNotFound
func IsNotFound(err error) bool {
	return errors.Is(err, pebble.ErrNotFound)
}

// lists all keys as strings for prefix; returns all if prefix empty
func ListKeys(prefix string) ([]string, error) {
	if IndexDB == nil {
		return nil, fmt.Errorf("pebble not opened; call Open first")
	}
	var pfx []byte
	if prefix != "" {
		pfx = []byte(prefix)
	} else {
		pfx = nil
	}
	iter, err := IndexDB.NewIter(&pebble.IterOptions{})
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
	if IndexDB == nil {
		return "", fmt.Errorf("pebble not opened; call Open first")
	}
	v, closer, err := IndexDB.Get([]byte(key))
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

// stores arbitrary key/value (namespace caution: e.g. "idx:author:")
func SaveKey(key string, value []byte) error {
	if IndexDB == nil {
		return fmt.Errorf("pebble not opened; call Open first")
	}
	if err := IndexDB.Set([]byte(key), value, WriteOpt(true)); err != nil {
		logger.Error("save_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("save_key_ok", "key", key, "len", len(value))
	return nil
}

// returns iterator, caller must close
func DBIter() (*pebble.Iterator, error) {
	if IndexDB == nil {
		return nil, fmt.Errorf("pebble not opened; call Open first")
	}
	return IndexDB.NewIter(&pebble.IterOptions{})
}

// writes key (bytes) as is, for admin use
func DBSet(key, value []byte) error {
	if IndexDB == nil {
		return fmt.Errorf("pebble not opened; call Open first")
	}
	return IndexDB.Set(key, value, WriteOpt(true))
}

// removes key
func DeleteKey(key string) error {
	if IndexDB == nil {
		return fmt.Errorf("pebble not opened; call Open first")
	}
	if err := IndexDB.Delete([]byte(key), WriteOpt(true)); err != nil {
		logger.Error("delete_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("delete_key_ok", "key", key)
	return nil
}

// chooses sync/no-sync writeOptions, always disables sync if WAL disabled
func WriteOpt(requestSync bool) *pebble.WriteOptions {
	if requestSync && !IndexWALDisabled {
		return pebble.Sync
	}
	return pebble.NoSync
}
