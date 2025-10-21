package ingest

import (
	"sync"

	"progressdb/pkg/config"
	"progressdb/pkg/ingest/apply"
	"progressdb/pkg/ingest/compute"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
)

// Ingestor coordinates compute and apply workers.
type Ingestor struct {
	computeWorker *compute.ComputeWorker
	applyWorker   *apply.ApplyWorker
	computeBuf    chan types.BatchEntry
	stop          chan struct{}
	wg            sync.WaitGroup
}

// NewIngestor creates a new Ingestor with compute and apply workers.
func NewIngestor(q *queue.IngestQueue, cc config.ComputeConfig, ac config.ApplyConfig) *Ingestor {
	if cc.WorkerCount <= 0 {
		cc.WorkerCount = 1
	}
	applyWorkers := ac.WorkerCount
	if applyWorkers <= 0 {
		applyWorkers = 1
	}
	if ac.BatchSize <= 0 {
		ac.BatchSize = 10
	}

	computeBuf := make(chan types.BatchEntry, 1000) // buffer size

	computeWorker := compute.NewComputeWorker(q, computeBuf, cc.WorkerCount)
	applyWorker := apply.NewApplyWorker(computeBuf, applyWorkers, ac.BatchSize)

	return &Ingestor{
		computeWorker: computeWorker,
		applyWorker:   applyWorker,
		computeBuf:    computeBuf,
		stop:          make(chan struct{}),
	}
}

// Start begins the ingestor workers.
func (i *Ingestor) Start() {
	i.computeWorker.Start(i.stop)
	i.applyWorker.Start(i.stop)
}

// Stop shuts down the ingestor workers.
func (i *Ingestor) Stop() {
	close(i.stop)
	i.wg.Wait()
}
