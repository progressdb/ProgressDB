package indexdb

import (
	"errors"
	"fmt"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
)

var Client *pebble.DB
var StorePath string
var WALDisabled bool
var PendingWrites uint64

func Open(path string, storageWalEnabled bool, intakeWALEnabled bool) error {
	var err error
	// WAL is always enabled for data integrity
	// storageWalEnabled parameter is kept for backward compatibility but ignored
	opts := &pebble.Options{
		DisableWAL: false, // Always enable WAL for durability
	}
	WALDisabled = false // WAL is always enabled

	if !intakeWALEnabled {
		logger.Warn("intake_wal_disabled", "durability", "intake WAL disabled but storage WAL enabled for index DB")
	}

	Client, err = pebble.Open(path, opts)
	if err != nil {
		logger.Error("pebble_open_failed", "path", path, "error", err)
		return err
	}
	StorePath = path
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
		return "", fmt.Errorf("pebble not opened; call Open first")
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
		return fmt.Errorf("pebble not opened; call Open first")
	}
	if err := Client.Set([]byte(key), value, WriteOpt(true)); err != nil {
		logger.Error("save_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("save_key_ok", "key", key, "len", len(value))
	return nil
}

func DBIter() (*pebble.Iterator, error) {
	if Client == nil {
		return nil, fmt.Errorf("pebble not opened; call Open first")
	}
	return Client.NewIter(&pebble.IterOptions{})
}

func DBSet(key, value []byte) error {
	if Client == nil {
		return fmt.Errorf("pebble not opened; call Open first")
	}
	return Client.Set(key, value, WriteOpt(true))
}

func DeleteKey(key string) error {
	if Client == nil {
		return fmt.Errorf("pebble not opened; call Open first")
	}
	if err := Client.Delete([]byte(key), WriteOpt(true)); err != nil {
		logger.Error("delete_key_failed", "key", key, "error", err)
		return err
	}
	logger.Debug("delete_key_ok", "key", key)
	return nil
}

func WriteOpt(requestSync bool) *pebble.WriteOptions {
	// WAL is always enabled, so we can safely sync when requested
	if requestSync {
		return pebble.Sync
	}
	return pebble.Sync
}

func ListKeysWithPrefixPaginated(prefix string, req *pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	if Client == nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("pebble not opened; call Open first")
	}

	iter, err := Client.NewIter(&pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: nextPrefix([]byte(prefix)),
	})
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	var keys []string
	count := 0

	// Simple forward iteration from prefix
	for iter.SeekGE([]byte(prefix)); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		keys = append(keys, key)
		count++

		if count >= req.Limit {
			break
		}
	}

	hasMore := iter.Valid() && len(keys) > 0

	response := pagination.PaginationResponse{
		HasAfter: hasMore,
		OrderBy:  req.OrderBy,
		Count:    len(keys),
		Total:    len(keys), // We don't have total count without scanning all
	}

	return keys, response, iter.Error()
}

// nextPrefix returns the next prefix after the given prefix for range scanning
func nextPrefix(prefix []byte) []byte {
	next := make([]byte, len(prefix))
	copy(next, prefix)
	for i := len(next) - 1; i >= 0; i-- {
		if next[i] < 0xff {
			next[i]++
			return next[:i+1]
		}
	}
	return prefix // overflow, return original
}
