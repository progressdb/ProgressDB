package storedb

import (
	"fmt"
	"sync/atomic"
	"time"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/db/index"

	"github.com/cockroachdb/pebble"
)

var PendingWrites uint64

// applies batch; sync forces fsync if true, else async write
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
		atomic.AddUint64(&PendingWrites, 1)
	}
	return err
}

// increments pending write counter by n
func RecordWrite(n int) {
	if n <= 0 {
		return
	}
	atomic.AddUint64(&PendingWrites, uint64(n))
}

// returns count of pending writes since last sync
func GetPendingWrites() uint64 {
	return atomic.LoadUint64(&PendingWrites)
}

// resets pending write counter
func ResetPendingWrites() {
	atomic.StoreUint64(&PendingWrites, 0)
}

// resets pending write counter
func ResetIndexPendingWrites() {
	atomic.StoreUint64(&index.IndexPendingWrites, 0)
}

// writes marker key, forces WAL fsync unless disabled (group-commit)
func ForceSync() error {
	if Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	if walDisabled {
		logger.Debug("pebble_force_sync_noop_wal_disabled")
		return nil
	}
	key := []byte("__progressDB_wal_sync_marker__")
	val := []byte(time.Now().UTC().Format(time.RFC3339Nano))
	if err := Client.Set(key, val, WriteOpt(true)); err != nil {
		logger.Error("pebble_force_sync_failed", "err", err)
		return err
	}
	return nil
}

// chooses sync/no-sync writeOptions, always disables sync if WAL disabled
func WriteOpt(requestSync bool) *pebble.WriteOptions {
	if requestSync && !walDisabled {
		return pebble.Sync
	}
	return pebble.NoSync
}

// applies batch; sync forces fsync if true, else async write
func ApplyIndexBatch(batch *pebble.Batch, sync bool) error {
	if index.IndexDB == nil {
		return fmt.Errorf("index pebble not opened; call index.Open first")
	}
	var err error
	err = index.IndexDB.Apply(batch, index.IndexWriteOpt(sync))
	if err != nil {
		logger.Error("index_pebble_apply_batch_failed", "error", err)
	}
	if err == nil {
		atomic.AddUint64(&index.IndexPendingWrites, 1)
	}
	return err
}

// increments pending write counter by n
func RecordIndexWrite(n int) {
	if n <= 0 {
		return
	}
	atomic.AddUint64(&index.IndexPendingWrites, uint64(n))
}

// returns count of pending writes since last sync
func GetIndexPendingWrites() uint64 {
	return atomic.LoadUint64(&index.IndexPendingWrites)
}

// writes marker key, forces WAL fsync unless disabled (group-commit)
func ForceIndexSync() error {
	if index.IndexDB == nil {
		return fmt.Errorf("index pebble not opened; call index.Open first")
	}
	if index.IndexWALDisabled {
		logger.Debug("index_pebble_force_sync_noop_wal_disabled")
		return nil
	}
	key := []byte("__progressDB_index_wal_sync_marker__")
	val := []byte(time.Now().UTC().Format(time.RFC3339Nano))
	if err := index.IndexDB.Set(key, val, index.IndexWriteOpt(true)); err != nil {
		logger.Error("index_pebble_force_sync_failed", "err", err)
		return err
	}
	return nil
}
