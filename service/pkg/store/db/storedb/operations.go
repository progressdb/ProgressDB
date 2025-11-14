package storedb

import (
	"fmt"
	"time"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/timeutil"

	"github.com/cockroachdb/pebble"
)

func ForceSync() error {
	if Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	if walDisabled {
		logger.Debug("pebble_force_sync_noop_wal_disabled")
		return nil
	}
	key := []byte("__progressDB_wal_sync_marker__")
	val := []byte(timeutil.Now().Format(time.RFC3339Nano))
	if err := Client.Set(key, val, WriteOpt(true)); err != nil {
		logger.Error("pebble_force_sync_failed", "err", err)
		return err
	}
	return nil
}

func WriteOpt(requestSync bool) *pebble.WriteOptions {
	if requestSync && !walDisabled {
		return pebble.Sync
	}
	return pebble.NoSync
}

func SetIndexKey(key, val []byte) error {
	if indexdb.Client == nil {
		return fmt.Errorf("pebble not opened; call Open first")
	}
	if indexdb.WALDisabled {
		return indexdb.Client.Set(key, val, pebble.NoSync)
	}
	if err := indexdb.Client.Set(key, val, indexdb.WriteOpt(true)); err != nil {
		logger.Error("set_key_failed", "error", err)
		return err
	}
	return nil
}
