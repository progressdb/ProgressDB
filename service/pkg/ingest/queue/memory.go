package queue

import (
	"fmt"
	"path/filepath"
	"time"

	"progressdb/pkg/config"
)

// NewIngestQueue creates a bounded IngestQueue of given capacity (>0).
func NewIngestQueue(capacity int) *IngestQueue {
	if capacity <= 0 {
		panic("queue.NewIngestQueue: capacity must be > 0; ensure config.ValidateConfig() applied defaults")
	}
	return &IngestQueue{
		ch:                make(chan *QueueItem, capacity),
		capacity:          capacity,
		drainPollInterval: 250 * time.Microsecond, // default
	}
}

// NewQueueFromConfig creates a queue based on the provided configuration.
// It handles both durable and memory modes using the config types directly.
func NewQueueFromConfig(qc config.QueueConfig, dbPath string) (*IngestQueue, error) {
	if qc.Mode == "durable" {
		// Create WAL directory path
		walDir := filepath.Join(dbPath, "wal")

		// Create WAL config from durable config
		walOpts := DurableWALConfigOptions{
			Dir:                 walDir,
			MaxFileSize:         qc.Durable.MaxFileSize.Int64(),
			EnableBatching:      qc.Durable.EnableBatching,
			BatchSize:           qc.Durable.FlushBatchSize,
			BatchInterval:       qc.Durable.BatchInterval.Duration(),
			EnableCompression:   qc.Durable.EnableCompression,
			MinCompressionBytes: qc.Durable.MinCompressionBytes,
		}

		wal, err := New(walOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create WAL: %w", err)
		}

		// Create queue options
		qOpts := &IngestQueueOptions{
			BufferCapacity:    qc.BufferCapacity,
			WAL:               wal,
			WriteMode:         qc.Durable.WriteMode,
			EnableRecovery:    qc.Durable.EnableRecovery,
			TruncateInterval:  qc.Durable.TruncateInterval.Duration(),
			WalBacked:         true,
			DrainPollInterval: qc.Memory.PollInterval.Duration(),
		}

		queue := NewIngestQueueWithOptions(qOpts)

		// Start background truncation if enabled
		if qc.Durable.TruncateInterval.Duration() > 0 {
			go func() {
				ticker := time.NewTicker(qc.Durable.TruncateInterval.Duration())
				defer ticker.Stop()
				for range ticker.C {
					queue.doTruncate()
				}
			}()
		}

		return queue, nil
	} else {
		// Memory mode
		return NewIngestQueue(qc.BufferCapacity), nil
	}
}

// NewIngestQueueFromConfig constructs a IngestQueue from a typed `config.QueueConfig`.
// Callers should ensure `config.ValidateConfig()` was run so fields are populated.
// Deprecated: Use NewQueueFromConfig instead.
func NewIngestQueueFromConfig(qc config.QueueConfig) *IngestQueue {
	return NewIngestQueue(qc.BufferCapacity)
}

// SetDefaultIngestQueue sets the package default if q is non-nil.
func SetDefaultIngestQueue(q *IngestQueue) {
	if q != nil {
		DefaultIngestQueue = q
	}
}
