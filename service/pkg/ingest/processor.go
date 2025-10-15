package ingest

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"progressdb/pkg/config"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
	"progressdb/pkg/store"
)

// Processor orchestrates workers that consume from the API queue, invoke
// registered handlers and ensure items are released back to the pool.
type Processor struct {
	q        *queue.IngestQueue
	workers  int
	stop     chan struct{}
	wg       sync.WaitGroup
	running  int32
	handlers map[queue.HandlerID]ProcessorFunc

	// batch knobs (future)
	maxBatch int
	flushDur time.Duration
	// pause state
	paused int32

	seqCounter uint64
	nextCommit uint64
	commitMu   sync.Mutex
	commitCond *sync.Cond
}

// NewProcessor creates a new Processor attached to the provided queue.
// It expects a validated ProcessorConfig (defaults applied by config.ValidateConfig()).
func NewProcessor(q *queue.IngestQueue, pc config.ProcessorConfig) *Processor {
	if pc.Workers <= 0 {
		panic("processor.NewProcessor: workers must be > 0; ensure config.ValidateConfig() applied defaults")
	}
	p := &Processor{
		q:          q,
		workers:    pc.Workers,
		stop:       make(chan struct{}),
		handlers:   make(map[queue.HandlerID]ProcessorFunc),
		maxBatch:   pc.MaxBatchMsgs,
		flushDur:   pc.FlushInterval.Duration(),
		nextCommit: 1,
	}
	p.commitCond = sync.NewCond(&p.commitMu)
	return p
}

// RegisterHandler registers a ProcessorFunc for a given HandlerID.
func (p *Processor) RegisterHandler(h queue.HandlerID, fn ProcessorFunc) {
	p.handlers[h] = fn
}

// SetBatchParams adjusts the batch parameters at runtime.
func (p *Processor) SetBatchParams(maxMsgs int, flush time.Duration) {
	if maxMsgs > 0 {
		p.maxBatch = maxMsgs
	}
	if flush > 0 {
		p.flushDur = flush
	}
}

// GetBatchParams returns the current batch parameters (max messages and flush duration).
func (p *Processor) GetBatchParams() (int, time.Duration) {
	return p.maxBatch, p.flushDur
}

// Pause stops processing new items until Resume is called.
func (p *Processor) Pause() {
	atomic.StoreInt32(&p.paused, 1)
}

// Resume resumes processing after a Pause.
func (p *Processor) Resume() {
	atomic.StoreInt32(&p.paused, 0)
}

// Start launches the worker pool.
func (p *Processor) Start() {
	if !atomic.CompareAndSwapInt32(&p.running, 0, 1) {
		return
	}
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go func(workerID int) {
			defer p.wg.Done()
			p.workerLoop(workerID)
		}(i)
	}
	logger.Info("ingest_processor_started", "workers", p.workers)
}

// Stop signals workers to exit and waits for them to finish.
func (p *Processor) Stop(ctx context.Context) {
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
		logger.Info("ingest_processor_stopped")
	case <-ctx.Done():
		logger.Warn("ingest_processor_stop_timeout")
	}
}

// workerLoop consumes items and dispatches to handlers. It ensures Item.Done()
// is always called to return pooled resources.
func (p *Processor) workerLoop(workerID int) {
	for {
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
				return
			}
			items = append(items, it)
		case <-p.stop:
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
			default:
				// no immediate item; yield briefly to avoid busy-loop
				time.Sleep(50 * time.Microsecond)
			}
		}
		drainTimer.Stop()

		// quick decode pass to extract thread IDs for prefetch
		threadSet := make(map[string]struct{})
		for _, it := range items {
			if it.Op.Thread != "" {
				threadSet[it.Op.Thread] = struct{}{}
				continue
			}
			// try light-weight JSON probe to find thread field
			var probe struct {
				Thread string `json:"thread"`
			}
			_ = json.Unmarshal(it.Op.Payload, &probe)
			if probe.Thread != "" {
				threadSet[probe.Thread] = struct{}{}
			}
		}

		// prefetch thread metadata in bulk
		threadMeta := make(map[string]string)
		for tid := range threadSet {
			if s, err := store.GetThread(tid); err == nil {
				threadMeta[tid] = s
			}
		}

		// process collected items in order: ask handlers for BatchEntries
		var batchEntries []BatchEntry
		for _, it := range items {
			// create handler context including prefetched thread metadata map
			hctx := context.WithValue(context.Background(), threadMetaKey, threadMeta)
			if fn, ok := p.handlers[it.Op.Handler]; ok && fn != nil {
				entries, err := fn(hctx, it.Op)
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

		// apply accumulated batch entries in commit order
		p.waitForCommit(seqID)
		if len(batchEntries) > 0 {
			if err := applyBatchToDB(batchEntries); err != nil {
				logger.Error("apply_batch_failed", "err", err)
				// TODO: retry / DLQ
			}
		}
		p.markCommitted(seqID)
	}
}

func (p *Processor) waitForCommit(seq uint64) {
	p.commitMu.Lock()
	for seq != p.nextCommit {
		p.commitCond.Wait()
	}
	p.commitMu.Unlock()
}

func (p *Processor) markCommitted(seq uint64) {
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
