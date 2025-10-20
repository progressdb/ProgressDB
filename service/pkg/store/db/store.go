package db

import (
	"bytes"
	"errors"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

var StoreDB *pebble.DB
var StoreDBPath string
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

	StoreDB, err = pebble.Open(path, opts)
	if err != nil {
		logger.Error("pebble_open_failed", "path", path, "error", err)
		return err
	}
	StoreDBPath = path
	return nil
}

// closes opened pebble StoreDB
func Close() error {
	if StoreDB == nil {
		return nil
	}
	if err := StoreDB.Close(); err != nil {
		return err
	}
	StoreDB = nil
	return nil
}

// returns true if StoreDB is opened
func Ready() bool {
	return StoreDB != nil
}

// returns true if error is pebble.ErrNotFound
func IsNotFound(err error) bool {
	return errors.Is(err, pebble.ErrNotFound)
}

// lists all keys as strings for prefix; returns all if prefix empty
func ListKeys(prefix string) ([]string, error) {
	tr := telemetry.Track("store.list_keys")
	defer tr.Finish()

	if StoreDB == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	var pfx []byte
	if prefix != "" {
		pfx = []byte(prefix)
	} else {
		pfx = nil
	}
	iter, err := StoreDB.NewIter(&pebble.IterOptions{})
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
	tr := telemetry.Track("store.get_key")
	defer tr.Finish()

	if StoreDB == nil {
		return "", fmt.Errorf("pebble not opened; call store.Open first")
	}
	tr.Mark("get")
	v, closer, err := StoreDB.Get([]byte(key))
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
	if StoreDB == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	if err := StoreDB.Set([]byte(key), value, WriteOpt(true)); err != nil {
		logger.Error("save_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("save_key_ok", "key", key, "len", len(value))
	return nil
}

// returns iterator, caller must close
func StoreDBIter() (*pebble.Iterator, error) {
	if StoreDB == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	return StoreDB.NewIter(&pebble.IterOptions{})
}

// writes key (bytes) as is, for admin use
func StoreDBSet(key, value []byte) error {
	if StoreDB == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	return StoreDB.Set(key, value, WriteOpt(true))
}

// removes key
func DeleteKey(key string) error {
	if StoreDB == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	if err := StoreDB.Delete([]byte(key), WriteOpt(true)); err != nil {
		logger.Error("delete_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("delete_key_ok", "key", key)
	return nil
}
