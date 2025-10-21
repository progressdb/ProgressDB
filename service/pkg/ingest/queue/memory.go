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
func NewQueueFromConfig(ic config.IntakeConfig, dbPath string) (*IngestQueue, error) {
	queue := NewIngestQueue(ic.BufferCapacity)
	queue.drainPollInterval = ic.ShutdownPollInterval.Duration()

	if ic.WAL.Enabled {
		// Create WAL directory path
		walDir := filepath.Join(dbPath, "wal")

		// Create simple WAL with custom segment size
		opts := &Options{
			SegmentSize: int(ic.WAL.SegmentSize.Int64()),
		}
		wal, err := Open(walDir, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create WAL: %w", err)
		}

		queue.wal = wal
		queue.walBacked = true
	}

	return queue, nil
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

// DisableWAL disables WAL backing for enqueues.
func (q *IngestQueue) DisableWAL() {
	q.walBacked = false
}

// EnableWAL enables WAL backing for enqueues.
func (q *IngestQueue) EnableWAL() {
	q.walBacked = true
}

// WAL returns the WAL log, if any.
func (q *IngestQueue) WAL() *Log {
	if q.wal == nil {
		return nil
	}
	return q.wal.(*Log)
}
