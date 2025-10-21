package ingest

import (
	"context"
	"encoding/json"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"progressdb/pkg/config"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
	"progressdb/pkg/telemetry"
)

// ApplyBatch represents a batch of entries to be applied with its sequence ID.
type ApplyBatch struct {
	SeqID   uint64
	Entries []BatchEntry
	MaxEnq  uint64
}

// TODO: retry / DLQ

// Ingestor orchestrates workers that consume from the API queue, invoke
// registered handlers and ensure items are released back to the pool.
type Ingestor struct {
	q        *queue.IngestQueue
	wal      *queue.Log // WAL for backup/replay
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

	// sequenced apply queue for memory mode
	applyCh        chan *ApplyBatch
	nextApplySeq   uint64
	pendingApplies map[uint64]*ApplyBatch
	applyMu        sync.Mutex
}

// NewIngestor creates a new Ingestor attached to the provided queue.
// It expects a validated IngestorConfig and QueueConfig (defaults applied by config.ValidateConfig()).
func NewIngestor(q *queue.IngestQueue, pc config.IngestorConfig, qc config.QueueConfig, walPath string) *Ingestor {
	if pc.WorkerCount <= 0 {
		panic("ingestor.NewIngestor: workers must be > 0; ensure config.ValidateConfig() applied defaults")
	}
	flushMs := pc.FlushIntervalMs
	maxBatch := pc.MaxBatchSize
	var wal *queue.Log
	if walPath != "" {
		// Open WAL for backup
		w, err := queue.Open(walPath, queue.DefaultOptions)
		if err != nil {
			panic("ingestor.NewIngestor: failed to open WAL: " + err.Error())
		}
		wal = w
	}
	p := &Ingestor{
		q:              q,
		wal:            wal,
		workers:        pc.WorkerCount,
		stop:           make(chan struct{}),
		handlers:       make(map[queue.HandlerID]IngestorFunc),
		maxBatch:       maxBatch,
		flushDur:       time.Duration(flushMs) * time.Millisecond,
		isMemory:       false,
		nextCommit:     1,
		applyCh:        make(chan *ApplyBatch, pc.ApplyQueueBufferSize), // configurable buffer to avoid blocking workers
		nextApplySeq:   1,
		pendingApplies: make(map[uint64]*ApplyBatch),
	}
	p.commitCond = sync.NewCond(&p.commitMu)
	// Replay WAL to memory queue on startup if WAL enabled
	if p.wal != nil {
		p.replayWALToQueue()
	}
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
	// start apply loop
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.applyLoop()
	}()
	// start n workers loops, pinned to OS threads
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go func(workerID int) {
			defer p.wg.Done()
			runtime.LockOSThread()
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
		if err := p.wal.Close(); err != nil {
			logger.Error("wal_close_failed", "err", err)
		}
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

		// find max EnqSeq in batch for checkpoint
		var maxEnq uint64
		for _, e := range batchEntries {
			if e.Enq > maxEnq {
				maxEnq = e.Enq
			}
		}

		// send to sequenced apply queue
		ab := &ApplyBatch{SeqID: seqID, Entries: batchEntries, MaxEnq: maxEnq}
		select {
		case p.applyCh <- ab:
			// sent successfully
		case <-p.stop:
			// stopping, apply directly to avoid loss
			p.applyBatch(ab)
			return
		default:
			if p.isMemory {
				// channel full, apply directly to avoid blocking (memory mode doesn't require strict ordering)
				logger.Warn("apply_channel_full", "applying_directly", seqID)
				p.applyBatch(ab)
			} else {
				// durable mode: wait for space to maintain ordering and prevent data loss
				select {
				case p.applyCh <- ab:
					// sent successfully after waiting
				case <-p.stop:
					p.applyBatch(ab)
					return
				}
			}
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

// applyLoop processes ApplyBatches in sequence for memory mode.
func (p *Ingestor) applyLoop() {
	for {
		select {
		case ab := <-p.applyCh:
			p.applyMu.Lock()
			if ab.SeqID == p.nextApplySeq {
				p.applyBatch(ab)
				p.nextApplySeq++
				// process any pending batches in sequence
				for {
					if next, ok := p.pendingApplies[p.nextApplySeq]; ok {
						delete(p.pendingApplies, p.nextApplySeq)
						p.applyBatch(next)
						p.nextApplySeq++
					} else {
						break
					}
				}
			} else if ab.SeqID > p.nextApplySeq {
				p.pendingApplies[ab.SeqID] = ab
			} // ignore if SeqID < nextApplySeq, already applied
			p.applyMu.Unlock()
		case <-p.stop:
			// drain remaining batches
			close(p.applyCh)
			for ab := range p.applyCh {
				p.applyBatch(ab) // apply remaining without sequencing, since stopping
			}
			return
		}
	}
}

// replayWALToQueue replays WAL entries into the memory queue on startup.
func (p *Ingestor) replayWALToQueue() {
	first, err := p.wal.FirstIndex()
	if err != nil {
		logger.Info("wal_replay_no_entries")
		return
	}
	last, err := p.wal.LastIndex()
	if err != nil {
		logger.Error("wal_replay_last_index_failed", "err", err)
		return
	}
	logger.Info("wal_replay_starting", "first", first, "last", last)
	for i := first; i <= last; i++ {
		data, err := p.wal.Read(i)
		if err != nil {
			logger.Error("wal_replay_read_failed", "index", i, "err", err)
			continue
		}
		// Assume data is QueueOp marshaled; unmarshal and enqueue to memory
		var op queue.QueueOp
		if err := json.Unmarshal(data, &op); err != nil {
			logger.Error("wal_replay_unmarshal_failed", "index", i, "err", err)
			continue
		}
		// Enqueue to memory (assuming queue supports direct enqueue)
		if err := p.q.Enqueue(context.Background(), &op); err != nil {
			logger.Error("wal_replay_enqueue_failed", "index", i, "err", err)
		}
	}
	logger.Info("wal_replay_completed")
}

// applyBatch applies a single ApplyBatch.
func (p *Ingestor) applyBatch(ab *ApplyBatch) {
	if len(ab.Entries) > 0 {
		tr := telemetry.Track("ingest.worker_batch_apply")
		if err := ApplyBatchToDB(ab.Entries); err != nil {
			logger.Error("apply_batch_failed", "err", err)
			// TODO: retry / DLQ
		} else {
			// Truncate WAL entries for this batch
			p.truncateWAL(ab)
		}
		tr.Finish()
	}
}

// truncateWAL deletes WAL entries from the batch's min to max Enq.
func (p *Ingestor) truncateWAL(ab *ApplyBatch) {
	if len(ab.Entries) == 0 {
		return
	}
	minEnq := ab.Entries[0].Enq
	maxEnq := ab.Entries[0].Enq
	for _, e := range ab.Entries {
		if e.Enq < minEnq {
			minEnq = e.Enq
		}
		if e.Enq > maxEnq {
			maxEnq = e.Enq
		}
	}
	// Assuming WAL index corresponds to Enq; truncate front to minEnq-1
	if minEnq > 1 {
		if err := p.wal.TruncateFront(minEnq - 1); err != nil {
			logger.Error("wal_truncate_front_failed", "err", err)
		}
	}
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
