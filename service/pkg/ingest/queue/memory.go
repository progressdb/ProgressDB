package queue

import (
	"progressdb/pkg/config"
)

// NewIngestQueue creates a bounded IngestQueue of given capacity (>0).
func NewIngestQueue(capacity int) *IngestQueue {
	if capacity <= 0 {
		panic("queue.NewIngestQueue: capacity must be > 0; ensure config.ValidateConfig() applied defaults")
	}
	return &IngestQueue{ch: make(chan *QueueItem, capacity), capacity: capacity}
}

// NewIngestQueueFromConfig constructs a IngestQueue from a typed `config.QueueConfig`.
// Callers should ensure `config.ValidateConfig()` was run so fields are populated.
func NewIngestQueueFromConfig(qc config.QueueConfig) *IngestQueue {
	return NewIngestQueue(qc.Capacity)
}

// SetDefaultIngestQueue sets the package default if q is non-nil.
func SetDefaultIngestQueue(q *IngestQueue) {
	if q != nil {
		DefaultIngestQueue = q
		// configure package backend to use the in-memory queue
		defaultBackend = q
	}
}
