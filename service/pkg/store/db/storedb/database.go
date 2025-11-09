package storedb

import (
	"errors"
	"fmt"

	"progressdb/pkg/state/logger"

	"github.com/cockroachdb/pebble"
)

var Client *pebble.DB
var StoreDBPath string
var walDisabled bool

func Open(path string, intakeWALEnabled bool) error {
	if Client != nil {
		return nil
	}
	var err error
	// WAL is always enabled for data integrity
	opts := &pebble.Options{
		DisableWAL: false, // Always enable WAL for durability
	}
	walDisabled = false // WAL is always enabled

	if !intakeWALEnabled {
		logger.Warn("intake_wal_disabled", "durability", "intake WAL disabled but storage WAL enabled")
	}

	Client, err = pebble.Open(path, opts)
	if err != nil {
		logger.Error("pebble_open_failed", "path", path, "error", err)
		return err
	}
	StoreDBPath = path
	return nil
}

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

func Ready() bool {
	return Client != nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, pebble.ErrNotFound)
}

func GetKey(key string) (string, error) {
	if Client == nil {
		return "", fmt.Errorf("pebble not opened; call db.Open first")
	}
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

func Iter() (*pebble.Iterator, error) {
	if Client == nil {
		return nil, fmt.Errorf("pebble not opened; call db.Open first")
	}
	return Client.NewIter(&pebble.IterOptions{})
}

func Set(key, value []byte) error {
	if Client == nil {
		return fmt.Errorf("pebble not opened; call db.Open first")
	}
	return Client.Set(key, value, WriteOpt(true))
}

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
