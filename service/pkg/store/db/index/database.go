package index

import (
	"bytes"
	"errors"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
)

var IndexDB *pebble.DB
var IndexDBPath string
var IndexWALDisabled bool
var IndexPendingWrites uint64

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

func Ready() bool {
	return IndexDB != nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, pebble.ErrNotFound)
}

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

func DBIter() (*pebble.Iterator, error) {
	if IndexDB == nil {
		return nil, fmt.Errorf("pebble not opened; call Open first")
	}
	return IndexDB.NewIter(&pebble.IterOptions{})
}

func DBSet(key, value []byte) error {
	if IndexDB == nil {
		return fmt.Errorf("pebble not opened; call Open first")
	}
	return IndexDB.Set(key, value, WriteOpt(true))
}

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

func WriteOpt(requestSync bool) *pebble.WriteOptions {
	if requestSync && !IndexWALDisabled {
		return pebble.Sync
	}
	return pebble.NoSync
}

func ListKeysPaginated(limit int, cursor string) ([]string, *pagination.PaginationResponse, error) {
	if IndexDB == nil {
		return nil, nil, fmt.Errorf("pebble not opened; call Open first")
	}

	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}

	iter, err := IndexDB.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer iter.Close()

	var out []string
	var startKey []byte

	if cursor != "" {
		// For index, cursor is just the last key seen
		decodedCursor, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}
		startKey = []byte(decodedCursor)
	}

	if startKey != nil {
		iter.SeekGE(startKey)
	} else {
		iter.First()
	}

	count := 0
	for iter.Valid() && count < limit {
		key := iter.Key()

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

	hasMore := iter.Valid()

	var nextCursor string
	if hasMore && len(out) > 0 {
		nextCursor = string(iter.Key())
	}

	return out, pagination.NewPaginationResponse(limit, hasMore, pagination.EncodeCursor(nextCursor), len(out)), iter.Error()
}

func ListKeysWithPrefixPaginated(prefix string, req *pagination.PaginationRequest) ([]string, *pagination.PaginationResponse, error) {
	if IndexDB == nil {
		return nil, nil, fmt.Errorf("pebble not opened; call db.Open first")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	// Allow larger limits for admin operations that need bulk data
	if limit > 1000 {
		limit = 1000
	}

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

	iter, err := IndexDB.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer iter.Close()

	var out []string
	var startKey []byte

	if req.Cursor != "" {
		// For index, cursor is just the last key seen
		decodedCursor, err := pagination.DecodeCursor(req.Cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}
		startKey = []byte(decodedCursor)
	} else {
		startKey = pfx
	}

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

		if req.Cursor != "" && count == 0 && bytes.Equal(key, startKey) {
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

	hasMore := iter.Valid() && (pfx == nil || bytes.HasPrefix(iter.Key(), pfx))

	var nextCursor string
	if hasMore && len(out) > 0 {
		nextCursor = string(iter.Key())
	}

	return out, pagination.NewPaginationResponse(limit, hasMore, pagination.EncodeCursor(nextCursor), len(out)), iter.Error()
}
