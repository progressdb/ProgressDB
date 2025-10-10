package queue

import (
	"time"
)

var activeWAL *FileWAL

// EnableDurable attempts to enable a FileWAL under the provided directory
// and replaces the package DefaultQueue with a durable-backed queue. Best
// effort: callers may ignore errors and continue with the in-memory queue.
func EnableDurable(dir string) error {
	if dir == "" {
		return nil
	}
	opts := Options{
		Dir:            dir,
		EnableBatch:    true,
		EnableCompress: false,
	}
	w, err := New(opts)
	if err != nil {
		return err
	}
	activeWAL = w

	qopts := &QueueOptions{
		Capacity:         defaultQueueCapacity,
		WAL:              w,
		Mode:             "batch",
		Recover:          true,
		TruncateInterval: 30 * time.Second,
	}
	q := NewQueueWithOptions(qopts)
	SetDefaultQueue(q)
	return nil
}

// CloseDurable closes the active WAL if any.
func CloseDurable() error {
	if activeWAL == nil {
		return nil
	}
	err := activeWAL.Close()
	activeWAL = nil
	return err
}
