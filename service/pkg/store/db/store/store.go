package storedb

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
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

// ListKeysPaginated lists keys with cursor-based pagination (required)
func ListKeys(prefix string, limit int, cursor string) ([]string, string, bool, error) {
	tr := telemetry.Track("db.list_keys_paginated")
	defer tr.Finish()

	if Client == nil {
		return nil, "", false, fmt.Errorf("pebble not opened; call db.Open first")
	}

	// Set default and max limits
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}

	var pfx []byte
	if prefix != "" {
		pfx = []byte(prefix)
	} else {
		pfx = nil
	}

	iter, err := Client.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, "", false, err
	}
	defer iter.Close()

	var out []string
	var startKey []byte

	// Decode cursor to get starting position
	if cursor != "" {
		decodedCursor, err := decodeStoreKeysCursor(cursor)
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor: %w", err)
		}
		startKey = []byte(decodedCursor.LastKey)
	} else {
		startKey = pfx
	}

	// Seek to start position
	if startKey != nil {
		iter.SeekGE(startKey)
	} else {
		iter.First()
	}

	count := 0
	for iter.Valid() && count < limit {
		key := iter.Key()
		if pfx != nil && !bytes.HasPrefix(key, pfx) {
			break
		}

		// Skip the cursor key itself when continuing pagination
		if cursor != "" && count == 0 && bytes.Equal(key, startKey) {
			iter.Next()
			continue
		}

		k := append([]byte(nil), key...)
		out = append(out, string(k))
		count++

		if count < limit {
			iter.Next()
		}
	}

	// Determine if more results exist
	hasMore := iter.Valid() && (pfx == nil || bytes.HasPrefix(iter.Key(), pfx))

	// Generate next cursor if we have more results
	var nextCursor string
	if hasMore && len(out) > 0 {
		nextCursor, err = encodeStoreKeysCursor(string(iter.Key()))
		if err != nil {
			return nil, "", false, fmt.Errorf("failed to encode cursor: %w", err)
		}
	}

	return out, nextCursor, hasMore, iter.Error()
}

// ListKeysPaginated lists keys with cursor-based pagination
func ListKeysPaginated(prefix string, limit int, cursor string) ([]string, string, bool, error) {
	tr := telemetry.Track("db.list_keys_paginated")
	defer tr.Finish()

	if Client == nil {
		return nil, "", false, fmt.Errorf("pebble not opened; call db.Open first")
	}

	// Set default and max limits
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}

	var pfx []byte
	if prefix != "" {
		pfx = []byte(prefix)
	} else {
		pfx = nil
	}

	iter, err := Client.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, "", false, err
	}
	defer iter.Close()

	var out []string
	var startKey []byte

	// Decode cursor to get starting position
	if cursor != "" {
		decodedCursor, err := decodeStoreKeysCursor(cursor)
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor: %w", err)
		}
		startKey = []byte(decodedCursor.LastKey)
	} else {
		startKey = pfx
	}

	// Seek to start position
	if startKey != nil {
		iter.SeekGE(startKey)
	} else {
		iter.First()
	}

	count := 0
	for iter.Valid() && count < limit {
		key := iter.Key()
		if pfx != nil && !bytes.HasPrefix(key, pfx) {
			break
		}

		// Skip the cursor key itself when continuing pagination
		if cursor != "" && count == 0 && bytes.Equal(key, startKey) {
			iter.Next()
			continue
		}

		k := append([]byte(nil), key...)
		out = append(out, string(k))
		count++

		if count < limit {
			iter.Next()
		}
	}

	// Determine if more results exist
	hasMore := iter.Valid() && (pfx == nil || bytes.HasPrefix(iter.Key(), pfx))

	// Generate next cursor if we have more results
	var nextCursor string
	if hasMore && len(out) > 0 {
		nextCursor, err = encodeStoreKeysCursor(string(iter.Key()))
		if err != nil {
			return nil, "", false, fmt.Errorf("failed to encode cursor: %w", err)
		}
	}

	return out, nextCursor, hasMore, iter.Error()
}

// encodeStoreKeysCursor creates a cursor for store keys pagination
func encodeStoreKeysCursor(lastKey string) (string, error) {
	cursor := map[string]interface{}{
		"last_key": lastKey,
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// decodeStoreKeysCursor decodes a cursor for store keys pagination
func decodeStoreKeysCursor(cursor string) (struct {
	LastKey string `json:"last_key"`
}, error) {
	var result struct {
		LastKey string `json:"last_key"`
	}

	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return result, err
	}

	err = json.Unmarshal(data, &result)
	return result, err
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
