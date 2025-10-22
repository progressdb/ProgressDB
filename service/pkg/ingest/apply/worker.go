package apply

import (
	"sort"
	"sync"
	"time"

	"progressdb/pkg/ingest/types"
	"progressdb/pkg/logger"
)

// ApplyWorker handles batching and applying types.BatchEntry to DB.
type ApplyWorker struct {
	input   <-chan types.BatchEntry
	stop    <-chan struct{}
	workers int
	buffer  []types.BatchEntry
	maxSize int
	timeout time.Duration
}

// NewApplyWorker creates a new apply worker.
func NewApplyWorker(input <-chan types.BatchEntry, workers, maxBatchSize int, timeout time.Duration) *ApplyWorker {
	return &ApplyWorker{
		input:   input,
		workers: workers,
		buffer:  make([]types.BatchEntry, 0, maxBatchSize),
		maxSize: maxBatchSize,
		timeout: timeout,
	}
}

// Start begins the apply workers.
func (aw *ApplyWorker) Start(stop <-chan struct{}, wg *sync.WaitGroup) {
	aw.stop = stop
	for i := 0; i < aw.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			aw.run()
		}()
	}
}

func (aw *ApplyWorker) run() {
	timer := time.NewTimer(aw.timeout)
	defer timer.Stop()

	for {
		select {
		case entry := <-aw.input:
			aw.buffer = append(aw.buffer, entry)
			if len(aw.buffer) >= aw.maxSize {
				aw.flush()
				timer.Reset(aw.timeout)
			}
		case <-timer.C:
			if len(aw.buffer) > 0 {
				aw.flush()
			}
			timer.Reset(aw.timeout)
		case <-aw.stop:
			aw.flush() // flush remaining
			return
		}
	}
}

func (aw *ApplyWorker) flush() {
	if len(aw.buffer) == 0 {
		return
	}
	// Sort buffer by TS ascending to ensure chronological order
	sort.Slice(aw.buffer, func(i, j int) bool {
		return aw.buffer[i].TS < aw.buffer[j].TS
	})
	if err := ApplyBatchToDB(aw.buffer); err != nil {
		logger.Error("apply_batch_failed", "err", err, "count", len(aw.buffer))
	}
	aw.buffer = aw.buffer[:0] // reset
}
