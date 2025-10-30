package ingest

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"progressdb/pkg/config"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

type Recovery struct {
	queue          *queue.IngestQueue
	mainDB         *pebble.DB
	indexDB        *pebble.DB
	enabled        bool
	walEnabled     bool
	tempIdxEnabled bool
}

type RecoveryStats struct {
	WALReplayed          int64         `json:"wal_replayed"`
	WALErrors            int64         `json:"wal_errors"`
	TempIndexesRecovered int64         `json:"temp_indexes_recovered"`
	TempIndexErrors      int64         `json:"temp_index_errors"`
	Duration             time.Duration `json:"duration"`
	Timestamp            time.Time     `json:"timestamp"`
}

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

	if r.walEnabled && r.queue.WAL() != nil {
		r.recoverWAL(stats)
	}

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

	if first == 0 && last == 0 {
		logger.Info("wal_empty", "nothing_to_recover")
		return
	}

	logger.Info("wal_recovery_range", "first", first, "last", last, "total_entries", last-first+1)

	replayedCount := int64(0)
	for i := first; i <= last; i++ {
		data, err := wal.Read(i)
		if err != nil {
			logger.Error("wal_recovery_read_error", "index", i, "error", err)
			stats.WALErrors++
			continue
		}

		var op queue.QueueOp
		if err := json.Unmarshal(data, &op); err != nil {
			logger.Error("wal_recovery_unmarshal_error", "index", i, "error", err)
			stats.WALErrors++
			continue
		}

		if err := r.queue.Enqueue(&op); err != nil {
			logger.Error("wal_recovery_enqueue_error", "index", i, "error", err)
			stats.WALErrors++
			continue
		}

		replayedCount++
	}

	stats.WALReplayed = replayedCount

	if replayedCount > 0 {
		if err := wal.TruncateFront(last + 1); err != nil {
			logger.Error("wal_recovery_truncate_error", "error", err)
			stats.WALErrors++
		} else {
			logger.Info("wal_recovery_truncated", "up_to_index", last+1)
		}
	}
}

func (r *Recovery) recoverTempIndexes(stats *RecoveryStats) {

	iter, err := r.mainDB.NewIter(&pebble.IterOptions{
		LowerBound: []byte(keys.TempIndexPrefix),
		UpperBound: []byte(keys.TempIndexUpperBound),
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

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		value := iter.Value()

		finalKey, indexData, err := r.parseTempIndexEntry(key, value)
		if err != nil {
			logger.Error("temp_index_recovery_parse_error", "key", key, "error", err)
			stats.TempIndexErrors++
			continue
		}

		indexBatch.Set([]byte(finalKey), indexData, nil)
		tempKeys = append(tempKeys, key)

		if len(tempKeys) >= batchSize {
			if err := r.applyIndexBatch(indexBatch, tempKeys, stats); err != nil {
				stats.TempIndexErrors++
			} else {
				recoveredCount += int64(len(tempKeys))
			}

			indexBatch.Close()
			indexBatch = r.indexDB.NewBatch()
			tempKeys = nil
		}
	}

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

func (r *Recovery) parseTempIndexEntry(tempKey string, tempValue []byte) (string, []byte, error) {
	parts := strings.SplitN(tempKey, ":", 3)
	if len(parts) != 3 || parts[0] != "temp_idx" {
		return "", nil, fmt.Errorf("invalid temp index key format: %s (expected %s)", tempKey, keys.TempIndexKeyFormat)
	}

	indexType := parts[1]
	targetKey := parts[2]

	finalKey := fmt.Sprintf(keys.RecoveryIndexKeyFormat, indexType, targetKey)

	return finalKey, tempValue, nil
}

func (r *Recovery) applyIndexBatch(indexBatch *pebble.Batch, tempKeys []string, stats *RecoveryStats) error {
	if err := r.indexDB.Apply(indexBatch, nil); err != nil {
		logger.Error("temp_index_recovery_apply_error", "error", err, "keys_count", len(tempKeys))
		return err
	}

	if err := r.cleanupTempKeys(tempKeys); err != nil {
		logger.Error("temp_index_recovery_cleanup_error", "error", err, "keys_count", len(tempKeys))
		return err
	}

	logger.Info("temp_index_recovery_batch_success", "keys_count", len(tempKeys))
	return nil
}

func (r *Recovery) cleanupTempKeys(tempKeys []string) error {
	mainBatch := r.mainDB.NewBatch()
	defer mainBatch.Close()

	for _, key := range tempKeys {
		mainBatch.Delete([]byte(key), nil)
	}

	return r.mainDB.Apply(mainBatch, nil)
}

var globalRecovery *Recovery

func InitGlobalRecovery(q *queue.IngestQueue, mainDB, indexDB *pebble.DB) {
	cfg := config.GetConfig()
	recoveryConfig := cfg.Ingest.Intake.Recovery
	intakeWALEnabled := cfg.Ingest.Intake.WAL.Enabled

	globalRecovery = NewRecovery(
		q,
		mainDB,
		indexDB,
		recoveryConfig.Enabled,
		recoveryConfig.WALEnabled && intakeWALEnabled,
		recoveryConfig.TempIdxEnabled,
	)
}

func RunGlobalRecovery() *RecoveryStats {
	if globalRecovery == nil {
		logger.Error("global_recovery_not_initialized")
		return &RecoveryStats{Timestamp: time.Now()}
	}
	return globalRecovery.RunRecovery()
}
