package storedb

import (
	"fmt"
	"sync/atomic"
	"time"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/timeutil"

	"github.com/cockroachdb/pebble"
)

var StorePendingWrites uint64

func ApplyBatch(batch *pebble.Batch, sync bool) error {
	if Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	var err error
	err = Client.Apply(batch, WriteOpt(sync))
	if err != nil {
		logger.Error("pebble_apply_batch_failed", "error", err)
	}
	if err == nil {
		atomic.AddUint64(&StorePendingWrites, 1)
	}
	return err
}

func RecordWrite(n int) {
	if n <= 0 {
		return
	}
	atomic.AddUint64(&StorePendingWrites, uint64(n))
}

func GetPendingWrites() uint64 {
	return atomic.LoadUint64(&StorePendingWrites)
}

func ResetPendingWrites() {
	atomic.StoreUint64(&StorePendingWrites, 0)
}

func ResetIndexPendingWrites() {
	atomic.StoreUint64(&indexdb.PendingWrites, 0)
}

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

func ApplyIndexBatch(batch *pebble.Batch, sync bool) error {
	if indexdb.Client == nil {
		return fmt.Errorf("pebble not opened; call Open first")
	}
	err := indexdb.Client.Apply(batch, indexdb.WriteOpt(sync))
	if err != nil {
		return err
	}
	atomic.AddUint64(&indexdb.PendingWrites, 1)
	return nil
}

func RecordIndexWrite(n int) {
	if n <= 0 {
		return
	}
	atomic.AddUint64(&indexdb.PendingWrites, uint64(n))
}

func PendingWrites() uint64 {
	return atomic.LoadUint64(&indexdb.PendingWrites)
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

func GetIndexPendingWrites() uint64 {
	return atomic.LoadUint64(&indexdb.PendingWrites)
}
