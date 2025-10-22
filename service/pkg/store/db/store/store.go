package storedb

import (
	"bytes"
	"errors"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

var Client *pebble.DB
var StoreDBPath string // Leave this alone as instructed
var walDisabled bool

// opens/creates pebble Client with WAL settings
func Open(path string, disablePebbleWAL bool, appWALEnabled bool) error {
	if Client != nil {
		return nil // already opened
	}
	var err error
	opts := &pebble.Options{
		DisableWAL: disablePebbleWAL,
	}
	walDisabled = opts.DisableWAL

	if walDisabled && !appWALEnabled {
		logger.Warn("durability_disabled", "durability", "no WAL enabled")
	}

	Client, err = pebble.Open(path, opts)
	if err != nil {
		logger.Error("pebble_open_failed", "path", path, "error", err)
		return err
	}
	StoreDBPath = path // keep this unchanged
	return nil
}

// closes opened pebble Client
func Close() error {
	if Client == nil {
		return nil
	}
	if err := Client.Close(); err != nil {
		return err
	}
	Client = nil
	return nil
}

// returns true if Client is opened
func Ready() bool {
	return Client != nil
}

// returns true if error is pebble.ErrNotFound
func IsNotFound(err error) bool {
	return errors.Is(err, pebble.ErrNotFound)
}

// lists all keys as strings for prefix; returns all if prefix empty
func ListKeys(prefix string) ([]string, error) {
	tr := telemetry.Track("db.list_keys")
	defer tr.Finish()

	if Client == nil {
		return nil, fmt.Errorf("pebble not opened; call db.Open first")
	}
	var pfx []byte
	if prefix != "" {
		pfx = []byte(prefix)
	} else {
		pfx = nil
	}
	iter, err := Client.NewIter(&pebble.IterOptions{})
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
	tr := telemetry.TrackWithStrategy("db.get_key", telemetry.RotationStrategyPurge)
	defer tr.Finish()

	if Client == nil {
		return "", fmt.Errorf("pebble not opened; call db.Open first")
	}
	tr.Mark("get")
	v, closer, err := Client.Get([]byte(key))
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
	if Client == nil {
		return fmt.Errorf("pebble not opened; call db.Open first")
	}
	if err := Client.Set([]byte(key), value, WriteOpt(true)); err != nil {
		logger.Error("save_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("save_key_ok", "key", key, "len", len(value))
	return nil
}

// returns iterator, caller must close
func Iter() (*pebble.Iterator, error) {
	if Client == nil {
		return nil, fmt.Errorf("pebble not opened; call db.Open first")
	}
	return Client.NewIter(&pebble.IterOptions{})
}

// writes key (bytes) as is, for admin use
func Set(key, value []byte) error {
	if Client == nil {
		return fmt.Errorf("pebble not opened; call db.Open first")
	}
	return Client.Set(key, value, WriteOpt(true))
}

// removes key
func DeleteKey(key string) error {
	if Client == nil {
		return fmt.Errorf("pebble not opened; call db.Open first")
	}
	if err := Client.Delete([]byte(key), WriteOpt(true)); err != nil {
		logger.Error("delete_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("delete_key_ok", "key", key)
	return nil
}
