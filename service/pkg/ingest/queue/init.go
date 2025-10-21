package queue

import (
	"fmt"
	"path/filepath"
	"time"

	"progressdb/pkg/config"
)

// GlobalIngestQueue is the global ingest queue instance.
var GlobalIngestQueue *IngestQueue

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

// InitGlobalIngestQueue creates and sets the global ingest queue with WAL enabled.
func InitGlobalIngestQueue(ic config.IntakeConfig, dbPath string) error {
	queue := NewIngestQueue(ic.BufferCapacity)
	queue.drainPollInterval = ic.ShutdownPollInterval.Duration()

	// Always enable WAL for durability
	walDir := filepath.Join(dbPath, "wal")
	opts := &Options{
		SegmentSize: int(ic.WAL.SegmentSize.Int64()),
	}
	wal, err := Open(walDir, opts)
	if err != nil {
		return fmt.Errorf("failed to create WAL: %w", err)
	}
	queue.wal = wal
	queue.walBacked = true

	GlobalIngestQueue = queue
	return nil
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
