package ingest

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
)

// Recovery handles crash recovery for both WAL entries and temp index data
type Recovery struct {
	queue          *queue.IngestQueue
	mainDB         *pebble.DB
	indexDB        *pebble.DB
	enabled        bool
	walEnabled     bool
	tempIdxEnabled bool
}

// RecoveryStats tracks recovery operations
type RecoveryStats struct {
	WALReplayed          int64         `json:"wal_replayed"`
	WALErrors            int64         `json:"wal_errors"`
	TempIndexesRecovered int64         `json:"temp_indexes_recovered"`
	TempIndexErrors      int64         `json:"temp_index_errors"`
	Duration             time.Duration `json:"duration"`
	Timestamp            time.Time     `json:"timestamp"`
}

// NewRecovery creates a new recovery instance
func NewRecovery(q *queue.IngestQueue, mainDB, indexDB *pebble.DB, enabled, walEnabled, tempIdxEnabled bool) *Recovery {
	return &Recovery{
		queue:          q,
		mainDB:         mainDB,
		indexDB:        indexDB,
		enabled:        enabled,
		walEnabled:     walEnabled,
		tempIdxEnabled: tempIdxEnabled,
	}
}

// RunRecovery performs all recovery operations on service startup
func (r *Recovery) RunRecovery() *RecoveryStats {
	stats := &RecoveryStats{
		Timestamp: time.Now(),
	}

	if !r.enabled {
		logger.Info("recovery_disabled")
		return stats
	}

	logger.Info("recovery_started", "wal_enabled", r.walEnabled, "temp_index_enabled", r.tempIdxEnabled)

	start := time.Now()

	// 1. Recover WAL entries first (critical - these are accepted operations)
	if r.walEnabled && r.queue.WAL() != nil {
		r.recoverWAL(stats)
	}

	// 2. Recover temp index entries (important for consistency)
	if r.tempIdxEnabled && r.mainDB != nil && r.indexDB != nil {
		r.recoverTempIndexes(stats)
	}

	stats.Duration = time.Since(start)
	logger.Info("recovery_completed",
		"wal_replayed", stats.WALReplayed,
		"wal_errors", stats.WALErrors,
		"temp_indexes_recovered", stats.TempIndexesRecovered,
		"temp_index_errors", stats.TempIndexErrors,
		"duration_ms", stats.Duration.Milliseconds())

	return stats
}

// recoverWAL replays unprocessed WAL entries back into queue
func (r *Recovery) recoverWAL(stats *RecoveryStats) {
	wal := r.queue.WAL()

	first, err := wal.FirstIndex()
	if err != nil {
		logger.Error("wal_recovery_first_index_error", "error", err)
		stats.WALErrors++
		return
	}

	last, err := wal.LastIndex()
	if err != nil {
		logger.Error("wal_recovery_last_index_error", "error", err)
		stats.WALErrors++
		return
	}

	// Check if WAL is empty
	if first == 0 && last == 0 {
		logger.Info("wal_empty", "nothing_to_recover")
		return
	}

	logger.Info("wal_recovery_range", "first", first, "last", last, "total_entries", last-first+1)

	// Replay all WAL entries
	replayedCount := int64(0)
	for i := first; i <= last; i++ {
		data, err := wal.Read(i)
		if err != nil {
			logger.Error("wal_recovery_read_error", "index", i, "error", err)
			stats.WALErrors++
			continue
		}

		// Reconstruct QueueOp from WAL data
		var op queue.QueueOp
		if err := json.Unmarshal(data, &op); err != nil {
			logger.Error("wal_recovery_unmarshal_error", "index", i, "error", err)
			stats.WALErrors++
			continue
		}

		// Re-enqueue operation for processing
		if err := r.queue.Enqueue(&op); err != nil {
			logger.Error("wal_recovery_enqueue_error", "index", i, "error", err)
			stats.WALErrors++
			continue
		}

		replayedCount++
	}

	stats.WALReplayed = replayedCount

	// Truncate processed WAL entries to prevent re-replay
	if replayedCount > 0 {
		if err := wal.TruncateFront(last + 1); err != nil {
			logger.Error("wal_recovery_truncate_error", "error", err)
			stats.WALErrors++
		} else {
			logger.Info("wal_recovery_truncated", "up_to_index", last+1)
		}
	}
}

// recoverTempIndexes moves temp index entries from main DB to index DB
func (r *Recovery) recoverTempIndexes(stats *RecoveryStats) {
	// Constants for temp index keys
	const tempIdxPrefix = "temp_idx:"
	const tempIdxUpper = "temp_idx;" // ASCII semicolon > colon

	// Create iterator for temp index entries
	iter, err := r.mainDB.NewIter(&pebble.IterOptions{
		LowerBound: []byte(tempIdxPrefix),
		UpperBound: []byte(tempIdxUpper),
	})
	if err != nil {
		logger.Error("temp_index_recovery_iterator_error", "error", err)
		stats.TempIndexErrors++
		return
	}
	defer iter.Close()

	indexBatch := r.indexDB.NewBatch()
	defer indexBatch.Close()

	var tempKeys []string
	recoveredCount := int64(0)
	batchSize := 1000

	logger.Info("temp_index_recovery_started")

	// Scan for temp index entries
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		value := iter.Value()

		// Parse temp index entry
		finalKey, indexData, err := r.parseTempIndexEntry(key, value)
		if err != nil {
			logger.Error("temp_index_recovery_parse_error", "key", key, "error", err)
			stats.TempIndexErrors++
			continue
		}

		// Add to index batch
		indexBatch.Set([]byte(finalKey), indexData, nil)
		tempKeys = append(tempKeys, key)

		// Apply batch when it reaches size limit
		if len(tempKeys) >= batchSize {
			if err := r.applyIndexBatch(indexBatch, tempKeys, stats); err != nil {
				stats.TempIndexErrors++
			} else {
				recoveredCount += int64(len(tempKeys))
			}

			// Reset for next batch
			indexBatch.Close()
			indexBatch = r.indexDB.NewBatch()
			tempKeys = nil
		}
	}

	// Apply final batch if there are remaining entries
	if len(tempKeys) > 0 {
		if err := r.applyIndexBatch(indexBatch, tempKeys, stats); err != nil {
			stats.TempIndexErrors++
		} else {
			recoveredCount += int64(len(tempKeys))
		}
	}

	stats.TempIndexesRecovered = recoveredCount
	logger.Info("temp_index_recovery_completed", "recovered", recoveredCount)
}

// parseTempIndexEntry extracts final index key and data from temp entry
func (r *Recovery) parseTempIndexEntry(tempKey string, tempValue []byte) (string, []byte, error) {
	// Temp key format: "temp_idx:index_type:target_key"
	// Example: "temp_idx:user_threads:user123"

	parts := strings.SplitN(tempKey, ":", 3)
	if len(parts) != 3 || parts[0] != "temp_idx" {
		return "", nil, fmt.Errorf("invalid temp index key format: %s", tempKey)
	}

	indexType := parts[1]
	targetKey := parts[2]

	// Construct final index key
	finalKey := fmt.Sprintf("idx:%s:%s", indexType, targetKey)

	return finalKey, tempValue, nil
}

// applyIndexBatch applies index batch and cleans up temp keys
func (r *Recovery) applyIndexBatch(indexBatch *pebble.Batch, tempKeys []string, stats *RecoveryStats) error {
	// Apply to index DB
	if err := r.indexDB.Apply(indexBatch, nil); err != nil {
		logger.Error("temp_index_recovery_apply_error", "error", err, "keys_count", len(tempKeys))
		return err
	}

	// Cleanup temp keys from main DB
	if err := r.cleanupTempKeys(tempKeys); err != nil {
		logger.Error("temp_index_recovery_cleanup_error", "error", err, "keys_count", len(tempKeys))
		return err
	}

	logger.Info("temp_index_recovery_batch_success", "keys_count", len(tempKeys))
	return nil
}

// cleanupTempKeys removes processed temp keys from main DB
func (r *Recovery) cleanupTempKeys(tempKeys []string) error {
	mainBatch := r.mainDB.NewBatch()
	defer mainBatch.Close()

	// Add delete operations for all temp keys
	for _, key := range tempKeys {
		mainBatch.Delete([]byte(key), nil)
	}

	// Apply cleanup batch
	return r.mainDB.Apply(mainBatch, nil)
}

// Global recovery instance
var globalRecovery *Recovery

// InitGlobalRecovery initializes the global recovery instance
func InitGlobalRecovery(q *queue.IngestQueue, mainDB, indexDB *pebble.DB, enabled, walEnabled, tempIdxEnabled bool) {
	globalRecovery = NewRecovery(q, mainDB, indexDB, enabled, walEnabled, tempIdxEnabled)
}

// SetRecoveryQueue updates the queue reference for global recovery
func SetRecoveryQueue(q *queue.IngestQueue) {
	if globalRecovery != nil {
		globalRecovery.queue = q
	}
}

// RunGlobalRecovery runs recovery using the global instance
func RunGlobalRecovery() *RecoveryStats {
	if globalRecovery == nil {
		logger.Error("global_recovery_not_initialized")
		return &RecoveryStats{Timestamp: time.Now()}
	}
	return globalRecovery.RunRecovery()
}
