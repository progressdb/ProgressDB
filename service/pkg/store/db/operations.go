package db

import (
	"fmt"
	"sync/atomic"
	"time"

	"progressdb/pkg/logger"

	"github.com/cockroachdb/pebble"
)

var PendingWrites uint64

// applies batch; sync forces fsync if true, else async write
func ApplyBatch(batch *pebble.Batch, sync bool) error {
	if StoreDB == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	var err error
	err = StoreDB.Apply(batch, WriteOpt(sync))
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
	atomic.StoreUint64(&IndexPendingWrites, 0)
}

// writes marker key, forces WAL fsync unless disabled (group-commit)
func ForceSync() error {
	if StoreDB == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	if walDisabled {
		logger.Debug("pebble_force_sync_noop_wal_disabled")
		return nil
	}
	key := []byte("__progressDB_wal_sync_marker__")
	val := []byte(time.Now().UTC().Format(time.RFC3339Nano))
	if err := StoreDB.Set(key, val, WriteOpt(true)); err != nil {
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
	if IndexDB == nil {
		return fmt.Errorf("index pebble not opened; call index.Open first")
	}
	var err error
	err = IndexDB.Apply(batch, IndexWriteOpt(sync))
	if err != nil {
		logger.Error("index_pebble_apply_batch_failed", "error", err)
	}
	if err == nil {
		atomic.AddUint64(&IndexPendingWrites, 1)
	}
	return err
}

// increments pending write counter by n
func RecordIndexWrite(n int) {
	if n <= 0 {
		return
	}
	atomic.AddUint64(&IndexPendingWrites, uint64(n))
}

// returns count of pending writes since last sync
func GetIndexPendingWrites() uint64 {
	return atomic.LoadUint64(&IndexPendingWrites)
}

// writes marker key, forces WAL fsync unless disabled (group-commit)
func ForceIndexSync() error {
	if IndexDB == nil {
		return fmt.Errorf("index pebble not opened; call index.Open first")
	}
	if indexWALDisabled {
		logger.Debug("index_pebble_force_sync_noop_wal_disabled")
		return nil
	}
	key := []byte("__progressDB_index_wal_sync_marker__")
	val := []byte(time.Now().UTC().Format(time.RFC3339Nano))
	if err := IndexDB.Set(key, val, IndexWriteOpt(true)); err != nil {
		logger.Error("index_pebble_force_sync_failed", "err", err)
		return err
	}
	return nil
}

// chooses sync/no-sync writeOptions, always disables sync if WAL disabled
func IndexWriteOpt(requestSync bool) *pebble.WriteOptions {
	if requestSync && !indexWALDisabled {
		return pebble.Sync
	}
	return pebble.NoSync
}
