package wally

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"progressdb/pkg/config"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	indexdb "progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

var globalWALReplayer *WALReplayer

// IngestQueue interface to avoid circular import
type IngestQueue interface {
	WAL() types.WAL
	Enqueue(op *types.QueueOp) error
	EnqueueReplay(op *types.QueueOp) error
}

type WALReplayer struct {
	queue          IngestQueue
	enabled        bool
	walEnabled     bool
	tempIdxEnabled bool
}

type ReplayStats struct {
	WALReplayed          int64         `json:"wal_replayed"`
	WALErrors            int64         `json:"wal_errors"`
	TempIndexesRecovered int64         `json:"temp_indexes_recovered"`
	TempIndexErrors      int64         `json:"temp_index_errors"`
	Duration             time.Duration `json:"duration"`
	Timestamp            time.Time     `json:"timestamp"`
}

func NewWALReplayer(q IngestQueue, enabled, walEnabled, tempIdxEnabled bool) *WALReplayer {
	return &WALReplayer{
		queue:          q,
		enabled:        enabled,
		walEnabled:     walEnabled,
		tempIdxEnabled: tempIdxEnabled,
	}
}

func (r *WALReplayer) Run() *ReplayStats {
	stats := &ReplayStats{
		Timestamp: time.Now(),
	}

	if !r.enabled {
		logger.Info("replay_disabled")
		return stats
	}

	logger.Info("replay_started", "wal_enabled", r.walEnabled, "temp_index_enabled", r.tempIdxEnabled)

	start := time.Now()

	if r.walEnabled && r.queue.WAL() != nil {
		r.recoverWAL(stats)
	}

	if r.tempIdxEnabled {
		r.recoverTempIndexes(stats)
	}

	stats.Duration = time.Since(start)
	logger.Info("replay_completed",
		"wal_replayed", stats.WALReplayed,
		"wal_errors", stats.WALErrors,
		"temp_indexes_recovered", stats.TempIndexesRecovered,
		"temp_index_errors", stats.TempIndexErrors,
		"duration_ms", stats.Duration.Milliseconds())

	return stats
}

func (r *WALReplayer) recoverWAL(stats *ReplayStats) {
	wal := r.queue.WAL()

	first, err := wal.FirstIndex()
	if err != nil {
		logger.Error("wal_replay_first_index_error", "error", err)
		stats.WALErrors++
		return
	}

	last, err := wal.LastIndex()
	if err != nil {
		logger.Error("wal_replay_last_index_error", "error", err)
		stats.WALErrors++
		return
	}

	if first == 0 && last == 0 {
		logger.Info("wal_empty", "nothing_to_recover")
		return
	}

	logger.Info("wal_replay_range", "first", first, "last", last, "total_entries", last-first+1)

	replayedCount := int64(0)
	var replayedSeqs []uint64
	for i := first; i <= last; i++ {
		data, err := wal.Read(i)
		if err != nil {
			logger.Error("wal_replay_read_error", "index", i, "error", err)
			stats.WALErrors++
			continue
		}

		var op types.QueueOp
		if err := json.Unmarshal(data, &op); err != nil {
			logger.Error("wal_replay_unmarshal_error", "index", i, "error", err)
			stats.WALErrors++
			continue
		}

		// Fix payload type based on handler
		if err := r.convertPayloadToType(&op); err != nil {
			logger.Error("wal_replay_payload_fix_error", "index", i, "handler", op.Handler, "error", err)
			stats.WALErrors++
			continue
		}

		if err := r.queue.EnqueueReplay(&op); err != nil {
			logger.Error("wal_replay_enqueue_error", "index", i, "error", err)
			stats.WALErrors++
			continue
		}

		replayedCount++
		replayedSeqs = append(replayedSeqs, op.EnqSeq)
	}

	stats.WALReplayed = replayedCount

	if replayedCount > 0 {
		if err := wal.TruncateSequences(replayedSeqs); err != nil {
			logger.Error("wal_replay_truncate_error", "error", err, "seq_count", len(replayedSeqs))
			stats.WALErrors++
		} else {
			logger.Info("wal_replay_truncated", "seq_count", len(replayedSeqs))
		}
	}
}

func (r *WALReplayer) recoverTempIndexes(stats *ReplayStats) {

	iter, err := storedb.Client.NewIter(&pebble.IterOptions{
		LowerBound: []byte(keys.TempIndexPrefix),
		UpperBound: []byte(keys.TempIndexUpperBound),
	})
	if err != nil {
		logger.Error("temp_index_replay_iterator_error", "error", err)
		stats.TempIndexErrors++
		return
	}
	defer iter.Close()

	indexBatch := indexdb.Client.NewBatch()
	defer indexBatch.Close()

	var tempKeys []string
	recoveredCount := int64(0)
	batchSize := 1000

	logger.Info("temp_index_replay_started")

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		value := iter.Value()

		finalKey, indexData, err := r.parseTempIndexEntry(key, value)
		if err != nil {
			logger.Error("temp_index_replay_parse_error", "key", key, "error", err)
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
			indexBatch = indexdb.Client.NewBatch()
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
	logger.Info("temp_index_replay_completed", "recovered", recoveredCount)
}

func (r *WALReplayer) parseTempIndexEntry(tempKey string, tempValue []byte) (string, []byte, error) {
	parts := strings.SplitN(tempKey, ":", 3)
	if len(parts) != 3 || parts[0] != "temp_idx" {
		return "", nil, fmt.Errorf("invalid temp index key format: %s (expected %s)", tempKey, keys.TempIndexKeyFormat)
	}

	indexType := parts[1]
	targetKey := parts[2]

	finalKey := fmt.Sprintf(keys.RecoveryIndexKeyFormat, indexType, targetKey)

	return finalKey, tempValue, nil
}

func (r *WALReplayer) applyIndexBatch(indexBatch *pebble.Batch, tempKeys []string, stats *ReplayStats) error {
	if err := indexdb.Client.Apply(indexBatch, nil); err != nil {
		logger.Error("temp_index_replay_apply_error", "error", err, "keys_count", len(tempKeys))
		return err
	}

	if err := r.cleanupTempKeys(tempKeys); err != nil {
		logger.Error("temp_index_replay_cleanup_error", "error", err, "keys_count", len(tempKeys))
		return err
	}

	logger.Info("temp_index_replay_batch_success", "keys_count", len(tempKeys))
	return nil
}

func (r *WALReplayer) cleanupTempKeys(tempKeys []string) error {
	mainBatch := storedb.Client.NewBatch()
	defer mainBatch.Close()

	for _, key := range tempKeys {
		mainBatch.Delete([]byte(key), nil)
	}

	return storedb.Client.Apply(mainBatch, nil)
}

func (r *WALReplayer) convertPayloadToType(op *types.QueueOp) error {
	if op.Payload == nil {
		return nil
	}

	// Convert map[string]interface{} to JSON then to proper struct
	payloadMap, ok := op.Payload.(map[string]interface{})
	if !ok {
		// Already proper type, nothing to fix
		return nil
	}

	payloadJSON, err := json.Marshal(payloadMap)
	if err != nil {
		return fmt.Errorf("failed to marshal payload map: %w", err)
	}

	switch op.Handler {
	case types.HandlerMessageCreate, types.HandlerMessageUpdate:
		var msg models.Message
		if err := json.Unmarshal(payloadJSON, &msg); err != nil {
			return fmt.Errorf("failed to unmarshal payload as Message: %w", err)
		}
		op.Payload = &msg

	case types.HandlerMessageDelete:
		var msg models.MessageDeletePartial
		if err := json.Unmarshal(payloadJSON, &msg); err != nil {
			return fmt.Errorf("failed to unmarshal payload as MessageDeletePartial: %w", err)
		}
		op.Payload = &msg

	case types.HandlerThreadCreate, types.HandlerThreadUpdate:
		var thread models.Thread
		if err := json.Unmarshal(payloadJSON, &thread); err != nil {
			return fmt.Errorf("failed to unmarshal payload as Thread: %w", err)
		}
		op.Payload = &thread

	case types.HandlerThreadDelete:
		var thread models.ThreadDeletePartial
		if err := json.Unmarshal(payloadJSON, &thread); err != nil {
			return fmt.Errorf("failed to unmarshal payload as ThreadDeletePartial: %w", err)
		}
		op.Payload = &thread

	default:
		return fmt.Errorf("unknown handler type: %s", op.Handler)
	}

	return nil
}

func InitWALReplay(q IngestQueue) {
	cfg := config.GetConfig()
	recoveryConfig := cfg.Ingest.Intake.Recovery
	intakeWALEnabled := cfg.Ingest.Intake.WAL.Enabled

	globalWALReplayer = NewWALReplayer(
		q,
		recoveryConfig.Enabled,
		intakeWALEnabled,
		recoveryConfig.TempIdxEnabled,
	)
}

func ReplayWAL() *ReplayStats {
	if globalWALReplayer == nil {
		logger.Error("global_wal_replayer_not_initialized")
		return &ReplayStats{Timestamp: time.Now()}
	}
	replayStats := globalWALReplayer.Run()
	if replayStats.WALErrors > 0 || replayStats.TempIndexErrors > 0 {
		logger.Warn("replay_completed_with_errors",
			"wal_errors", replayStats.WALErrors,
			"temp_index_errors", replayStats.TempIndexErrors)
	}
	return replayStats
}
