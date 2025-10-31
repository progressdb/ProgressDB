package queue

import (
	"fmt"
	"path/filepath"
	"time"

	"progressdb/pkg/config"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/ingest/wally"
)

var GlobalIngestQueue *IngestQueue

func NewIngestQueue(capacity int) *IngestQueue {
	if capacity <= 0 {
		panic("queue.NewIngestQueue: capacity must be > 0; ensure config.ValidateConfig() applied defaults")
	}
	return &IngestQueue{
		ch:                make(chan *types.QueueItem, capacity),
		capacity:          capacity,
		drainPollInterval: 250 * time.Microsecond, // default
	}
}

func InitGlobalIngestQueue(dbPath string) error {
	cfg := config.GetConfig()
	ic := cfg.Ingest.Intake

	queue := NewIngestQueue(ic.QueueCapacity)
	queue.drainPollInterval = ic.ShutdownPollInterval.Duration()

	if ic.WAL.Enabled {
		// Enable WAL for durability
		walDir := filepath.Join(dbPath, "wal")
		wal, err := wally.Open(walDir)
		if err != nil {
			return fmt.Errorf("failed to create WAL: %w", err)
		}
		queue.wal = wal
		queue.intakeWalEnabled = true
	}

	GlobalIngestQueue = queue
	return nil
}

func (q *IngestQueue) DisableWAL() {
	q.intakeWalEnabled = false
}

func (q *IngestQueue) EnableWAL() {
	q.intakeWalEnabled = true
}

func (q *IngestQueue) WAL() types.WAL {
	return q.wal
}
