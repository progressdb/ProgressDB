package ingest

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"progressdb/pkg/config"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
	"progressdb/pkg/telemetry"
)

// TODO: retry / DLQ

// Ingestor orchestrates workers that consume from the API queue, invoke
// registered handlers and ensure items are released back to the pool.
type Ingestor struct {
	q        *queue.IngestQueue
	workers  int
	stop     chan struct{}
	wg       sync.WaitGroup
	running  int32
	handlers map[queue.HandlerID]IngestorFunc

	// batch knobs (future)
	maxBatch int
	flushDur time.Duration
	// pause state
	paused int32

	isMemory   bool
	seqCounter uint64
	nextCommit uint64
	commitMu   sync.Mutex
	commitCond *sync.Cond
}

// NewIngestor creates a new Ingestor attached to the provided queue.
// It expects a validated IngestorConfig and QueueConfig (defaults applied by config.ValidateConfig()).
func NewIngestor(q *queue.IngestQueue, pc config.IngestorConfig, qc config.QueueConfig) *Ingestor {
	if pc.WorkerCount <= 0 {
		panic("ingestor.NewIngestor: workers must be > 0; ensure config.ValidateConfig() applied defaults")
	}
	var flushMs, maxBatch int
	if qc.Mode == "memory" {
		flushMs = qc.Memory.FlushIntervalMs
		maxBatch = qc.Memory.FlushBatchSize
	} else {
		flushMs = qc.Durable.FlushIntervalMs
		maxBatch = qc.Durable.FlushBatchSize
	}
	p := &Ingestor{
		q:          q,
		workers:    pc.WorkerCount,
		stop:       make(chan struct{}),
		handlers:   make(map[queue.HandlerID]IngestorFunc),
		maxBatch:   maxBatch,
		flushDur:   time.Duration(flushMs) * time.Millisecond,
		isMemory:   qc.Mode == "memory",
		nextCommit: 1,
	}
	p.commitCond = sync.NewCond(&p.commitMu)
	registerDefaultHandlers(p)
	return p
}

// RegisterHandler registers a IngestorFunc for a given HandlerID.
func (p *Ingestor) RegisterHandler(h queue.HandlerID, fn IngestorFunc) {
	p.handlers[h] = fn
}

// SetBatchParams adjusts the batch parameters at runtime.
func (p *Ingestor) SetBatchParams(maxMsgs int, flush time.Duration) {
	if maxMsgs > 0 {
		p.maxBatch = maxMsgs
	}
	if flush > 0 {
		p.flushDur = flush
	}
}

// GetBatchParams returns the current batch parameters (max messages and flush duration).
func (p *Ingestor) GetBatchParams() (int, time.Duration) {
	return p.maxBatch, p.flushDur
}

// Pause stops processing new items until Resume is called.
func (p *Ingestor) Pause() {
	atomic.StoreInt32(&p.paused, 1)
}

// Resume resumes processing after a Pause.
func (p *Ingestor) Resume() {
	atomic.StoreInt32(&p.paused, 0)
}

// Start launches the worker pool.
func (p *Ingestor) Start() {
	if !atomic.CompareAndSwapInt32(&p.running, 0, 1) {
		return
	}
	// start n worksers loops
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go func(workerID int) {
			defer p.wg.Done()
			p.workerLoop(workerID)
		}(i)
	}
	logger.Info("ingestor_started", "workers", p.workers)
}

// Stop signals workers to exit and waits for them to finish.
func (p *Ingestor) Stop(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&p.running, 1, 0) {
		return
	}
	close(p.stop)
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		logger.Info("ingestor_stopped")
	case <-ctx.Done():
		logger.Warn("ingestor_stop_timeout")
	}
}

// workerLoop consumes items and dispatches to handlers. It ensures Item.Done()
// is always called to return pooled resources.
func (p *Ingestor) workerLoop(workerID int) {
	for {
		tr := telemetry.Track("ingest.worker_loop")
		// if paused, wait briefly and re-check
		if atomic.LoadInt32(&p.paused) == 1 {
			select {
			case <-p.stop:
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		// Batch collect
		var items []*queue.QueueItem
		// block for first item or stop
		select {
		case it, ok := <-p.q.Out():
			if !ok {
				tr.Finish()
				return
			}
			items = append(items, it)
		case <-p.stop:
			tr.Finish()
			return
		}

		seqID := atomic.AddUint64(&p.seqCounter, 1)

		// drain up to maxBatch or until flushDur
		drainTimer := time.NewTimer(p.flushDur)
	drainLoop:
		for len(items) < p.maxBatch {
			select {
			case it, ok := <-p.q.Out():
				if !ok {
					break drainLoop
				}
				items = append(items, it)
			case <-drainTimer.C:
				break drainLoop
			case <-p.stop:
				drainTimer.Stop()
				return
			}
		}
		drainTimer.Stop()

		// process collected items in order: ask handlers for BatchEntries
		var batchEntries []BatchEntry
		for _, it := range items {
			if fn, ok := p.handlers[it.Op.Handler]; ok && fn != nil {
				entries, err := fn(context.Background(), it.Op)
				if err != nil {
					logger.Error("ingest_handler_error", "handler", it.Op.Handler, "error", err)
					it.Done()
					continue
				}
				for i := range entries {
					if entries[i].Enq == 0 {
						entries[i].Enq = it.Op.EnqSeq
					}
				}
				batchEntries = append(batchEntries, entries...)
				// release pooled resources for this item now that we've copied
				// any necessary data into BatchEntries.
				it.Done()
			} else {
				logger.Warn("no_ingest_handler", "handler", it.Op.Handler)
				it.Done()
			}
		}

		// apply accumulated batch entries in commit order (skip for memory mode)
		if !p.isMemory {
			p.waitForCommit(seqID)
		}
		if len(batchEntries) > 0 {
			tr := telemetry.Track("ingest.worker_batch_apply")
			if err := ApplyBatchToDB(batchEntries); err != nil {
				logger.Error("apply_batch_failed", "err", err)
				// TODO: retry / DLQ
			} else {
				// Checkpoint successful apply
				p.q.Checkpoint(seqID)
				// Truncate WAL immediately after successful batch
				p.q.TruncateNow()
			}
			tr.Finish()
		}
		if !p.isMemory {
			p.markCommitted(seqID)
		}
		tr.Finish()
	}
}

func (p *Ingestor) waitForCommit(seq uint64) {
	p.commitMu.Lock()
	for seq != p.nextCommit {
		p.commitCond.Wait()
	}
	p.commitMu.Unlock()
}

func (p *Ingestor) markCommitted(seq uint64) {
	p.commitMu.Lock()
	if seq == p.nextCommit {
		p.nextCommit++
		p.commitCond.Broadcast()
	} else if seq > p.nextCommit {
		// Should not happen, but avoid deadlock if it does.
		p.nextCommit = seq + 1
		p.commitCond.Broadcast()
	}
	p.commitMu.Unlock()
}

// wires handlers per ops
func registerDefaultHandlers(p *Ingestor) {
	p.RegisterHandler(queue.HandlerMessageCreate, MutMessageCreate)
	p.RegisterHandler(queue.HandlerMessageUpdate, MutMessageUpdate)
	p.RegisterHandler(queue.HandlerMessageDelete, MutMessageDelete)
	p.RegisterHandler(queue.HandlerReactionAdd, MutReactionAdd)
	p.RegisterHandler(queue.HandlerReactionDelete, MutReactionDelete)
	p.RegisterHandler(queue.HandlerThreadCreate, MutThreadCreate)
	p.RegisterHandler(queue.HandlerThreadUpdate, MutThreadUpdate)
	p.RegisterHandler(queue.HandlerThreadDelete, MutThreadDelete)
}
