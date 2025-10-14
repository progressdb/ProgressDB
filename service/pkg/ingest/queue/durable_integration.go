package queue

import (
	"time"
)

var activeWAL *FileWAL

// DurableEnableOptions provide a richer shape for enabling durable WAL and
// constructing the canonical queue from configuration.
type DurableEnableOptions struct {
    Dir              string
    Capacity         int
    TruncateInterval time.Duration

    // WAL tunables
    WALMaxFileSize    int64
    WALEnableBatch    bool
    WALBatchSize      int
    WALBatchInterval  time.Duration
    WALEnableCompress bool
    WALCompressMinBytes int64
    WALCompressMinRatio float64
}

// EnableDurable attempts to enable a FileWAL under the provided options
// and replaces the package DefaultQueue with a durable-backed queue. Best
// effort: callers may ignore errors and continue with the in-memory queue.
func EnableDurable(opts DurableEnableOptions) error {
	if opts.Dir == "" {
		return nil
	}

    wopts := Options{
        Dir:            opts.Dir,
        MaxFileSize:    opts.WALMaxFileSize,
        EnableBatch:    opts.WALEnableBatch,
        BatchSize:      opts.WALBatchSize,
        BatchInterval:  opts.WALBatchInterval,
        EnableCompress: opts.WALEnableCompress,
        CompressMinBytes: opts.WALCompressMinBytes,
        CompressMinRatio: opts.WALCompressMinRatio,
    }
	w, err := New(wopts)
	if err != nil {
		return err
	}
	activeWAL = w
	qopts := &QueueOptions{
		Capacity:         opts.Capacity,
		WAL:              w,
		Mode:             "batch",
		Recover:          true,
		TruncateInterval: opts.TruncateInterval,
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
