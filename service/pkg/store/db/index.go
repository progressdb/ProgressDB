package store

import (
	"bytes"
	"errors"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

var indexDB *pebble.DB
var indexDBPath string
var indexWALDisabled bool
var indexPendingWrites uint64

// opens/creates pebble DB with WAL settings for index storage
func OpenIndex(path string, disablePebbleWAL bool, appWALEnabled bool) error {
	var err error
	opts := &pebble.Options{
		DisableWAL: disablePebbleWAL,
	}
	indexWALDisabled = opts.DisableWAL

	if indexWALDisabled && !appWALEnabled {
		logger.Warn("index_durability_disabled", "durability", "no WAL enabled for index DB")
	}

	indexDB, err = pebble.Open(path, opts)
	if err != nil {
		logger.Error("index_pebble_open_failed", "path", path, "error", err)
		return err
	}
	indexDBPath = path
	return nil
}

// closes opened pebble DB
func CloseIndex() error {
	if indexDB == nil {
		return nil
	}
	if err := indexDB.Close(); err != nil {
		return err
	}
	indexDB = nil
	return nil
}

// returns true if DB is opened
func IndexReady() bool {
	return indexDB != nil
}

// returns true if error is pebble.ErrNotFound
func IndexIsNotFound(err error) bool {
	return errors.Is(err, pebble.ErrNotFound)
}

// lists all keys as strings for prefix; returns all if prefix empty
func ListIndexKeys(prefix string) ([]string, error) {
	tr := telemetry.Track("index.list_keys")
	defer tr.Finish()

	if indexDB == nil {
		return nil, fmt.Errorf("index pebble not opened; call index.Open first")
	}
	var pfx []byte
	if prefix != "" {
		pfx = []byte(prefix)
	} else {
		pfx = nil
	}
	iter, err := indexDB.NewIter(&pebble.IterOptions{})
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
func GetIndexKey(key string) (string, error) {
	tr := telemetry.Track("index.get_key")
	defer tr.Finish()

	if indexDB == nil {
		return "", fmt.Errorf("index pebble not opened; call index.Open first")
	}
	tr.Mark("get")
	v, closer, err := indexDB.Get([]byte(key))
	if err != nil {
		if errors.Is(err, pebble.ErrNotFound) {
			logger.Debug("index_get_key_missing", "key", key)
		} else {
			logger.Error("index_get_key_failed", "key", key, "error", err)
		}
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}
	logger.Debug("index_get_key_ok", "key", key, "len", len(v))
	return string(v), nil
}

// stores arbitrary key/value (namespace caution: e.g. "idx:author:")
func SaveIndexKey(key string, value []byte) error {
	if indexDB == nil {
		return fmt.Errorf("index pebble not opened; call index.Open first")
	}
	if err := indexDB.Set([]byte(key), value, indexWriteOpt(true)); err != nil {
		logger.Error("index_save_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("index_save_key_ok", "key", key, "len", len(value))
	return nil
}

// returns iterator, caller must close
func IndexDBIter() (*pebble.Iterator, error) {
	if indexDB == nil {
		return nil, fmt.Errorf("index pebble not opened; call index.Open first")
	}
	return indexDB.NewIter(&pebble.IterOptions{})
}

// writes key (bytes) as is, for admin use
func IndexDBSet(key, value []byte) error {
	if indexDB == nil {
		return fmt.Errorf("index pebble not opened; call index.Open first")
	}
	return indexDB.Set(key, value, indexWriteOpt(true))
}

// removes key
func DeleteIndexKey(key string) error {
	if indexDB == nil {
		return fmt.Errorf("index pebble not opened; call index.Open first")
	}
	if err := indexDB.Delete([]byte(key), indexWriteOpt(true)); err != nil {
		logger.Error("index_delete_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("index_delete_key_ok", "key", key)
	return nil
}
