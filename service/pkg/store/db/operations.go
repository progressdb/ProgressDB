package store

import (
	"fmt"
	"sync/atomic"
	"time"

	"progressdb/pkg/logger"

	"github.com/cockroachdb/pebble"
)

var pendingWrites uint64

// applies batch; sync forces fsync if true, else async write
func ApplyBatch(batch *pebble.Batch, sync bool) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	var err error
	err = db.Apply(batch, writeOpt(sync))
	if err != nil {
		logger.Error("pebble_apply_batch_failed", "error", err)
	}
	if err == nil {
		atomic.AddUint64(&pendingWrites, 1)
	}
	return err
}

// increments pending write counter by n
func RecordWrite(n int) {
	if n <= 0 {
		return
	}
	atomic.AddUint64(&pendingWrites, uint64(n))
}

// returns count of pending writes since last sync
func GetPendingWrites() uint64 {
	return atomic.LoadUint64(&pendingWrites)
}

// resets pending write counter
func ResetPendingWrites() {
	atomic.StoreUint64(&pendingWrites, 0)
}

// writes marker key, forces WAL fsync unless disabled (group-commit)
func ForceSync() error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	if walDisabled {
		logger.Debug("pebble_force_sync_noop_wal_disabled")
		return nil
	}
	key := []byte("__progressdb_wal_sync_marker__")
	val := []byte(time.Now().UTC().Format(time.RFC3339Nano))
	if err := db.Set(key, val, writeOpt(true)); err != nil {
		logger.Error("pebble_force_sync_failed", "err", err)
		return err
	}
	return nil
}

// chooses sync/no-sync WriteOptions, always disables sync if WAL disabled
func writeOpt(requestSync bool) *pebble.WriteOptions {
	if requestSync && !walDisabled {
		return pebble.Sync
	}
	return pebble.NoSync
}

// applies batch; sync forces fsync if true, else async write
func ApplyIndexBatch(batch *pebble.Batch, sync bool) error {
	if indexDB == nil {
		return fmt.Errorf("index pebble not opened; call index.Open first")
	}
	var err error
	err = indexDB.Apply(batch, indexWriteOpt(sync))
	if err != nil {
		logger.Error("index_pebble_apply_batch_failed", "error", err)
	}
	if err == nil {
		atomic.AddUint64(&indexPendingWrites, 1)
	}
	return err
}

// increments pending write counter by n
func RecordIndexWrite(n int) {
	if n <= 0 {
		return
	}
	atomic.AddUint64(&indexPendingWrites, uint64(n))
}

// returns count of pending writes since last sync
func GetIndexPendingWrites() uint64 {
	return atomic.LoadUint64(&indexPendingWrites)
}

// resets pending write counter
func ResetIndexPendingWrites() {
	atomic.StoreUint64(&indexPendingWrites, 0)
}

// writes marker key, forces WAL fsync unless disabled (group-commit)
func ForceIndexSync() error {
	if indexDB == nil {
		return fmt.Errorf("index pebble not opened; call index.Open first")
	}
	if indexWALDisabled {
		logger.Debug("index_pebble_force_sync_noop_wal_disabled")
		return nil
	}
	key := []byte("__progressdb_index_wal_sync_marker__")
	val := []byte(time.Now().UTC().Format(time.RFC3339Nano))
	if err := indexDB.Set(key, val, indexWriteOpt(true)); err != nil {
		logger.Error("index_pebble_force_sync_failed", "err", err)
		return err
	}
	return nil
}

// chooses sync/no-sync WriteOptions, always disables sync if WAL disabled
func indexWriteOpt(requestSync bool) *pebble.WriteOptions {
	if requestSync && !indexWALDisabled {
		return pebble.Sync
	}
	return pebble.NoSync
}
