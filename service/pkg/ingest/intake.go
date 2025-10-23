package ingest

import (
	"sync"

	"progressdb/pkg/config"
	"progressdb/pkg/ingest/apply"
	"progressdb/pkg/ingest/compute"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/state"
)

// Ingestor coordinates compute and apply workers.
type Ingestor struct {
	computeWorker *compute.ComputeWorker
	applyWorker   *apply.ApplyWorker
	computeBuf    chan types.BatchEntry
	stop          chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
}

// NewIngestor creates a new Ingestor with compute and apply workers.
func NewIngestor(q *queue.IngestQueue, cc config.ComputeConfig, ac config.ApplyConfig, dbPath string) *Ingestor {
	if cc.WorkerCount <= 0 {
		cc.WorkerCount = 1
	}
	applyWorkers := 1 // fixed to 1 for sequencing
	if ac.BatchCount <= 0 {
		ac.BatchCount = 10
	}
	if cc.BufferCapacity <= 0 {
		cc.BufferCapacity = 1000
	}

	computeBuf := make(chan types.BatchEntry, cc.BufferCapacity)

	failedOpsPath := state.FailedOpsPath(dbPath)
	computeWorker := compute.NewComputeWorker(q, computeBuf, cc.WorkerCount, failedOpsPath)
	applyWorker := apply.NewApplyWorker(computeBuf, applyWorkers, ac.BatchCount, ac.BatchTimeout.Duration())

	return &Ingestor{
		computeWorker: computeWorker,
		applyWorker:   applyWorker,
		computeBuf:    computeBuf,
		stop:          make(chan struct{}),
	}
}

// Start begins the ingestor workers.
func (i *Ingestor) Start() {
	i.computeWorker.Start(i.stop, &i.wg)
	i.applyWorker.Start(i.stop, &i.wg)
}

// Stop shuts down the ingestor workers.
func (i *Ingestor) Stop() {
	i.stopOnce.Do(func() {
		close(i.stop)
	})
	i.wg.Wait()
}
