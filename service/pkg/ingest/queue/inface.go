package queue

import (
	"encoding/json"
	"errors"
	"sync/atomic"

	"progressdb/pkg/ingest/types"
)

// TODO: queue full - drop strategy
// TODO: handler error - handlng strategy

var (
	// enqueue sequence number
	enqSeq uint64
)

// Queue errors
var (
	ErrQueueFull   = errors.New("ingest queue full")
	ErrQueueClosed = errors.New("ingest queue closed")
)

// enqueue operation (non-blocking)
func (q *IngestQueue) Enqueue(op *types.QueueOp) error {
	return q.enqueue(op)
}

// internal enqueue function
func (q *IngestQueue) enqueue(op *types.QueueOp) error {
	// Acquire enqueue lock to atomically check closed state and increment WaitGroup
	q.enqMu.Lock()
	if atomic.LoadInt32(&q.closed) == 1 {
		q.enqMu.Unlock()
		return ErrQueueClosed
	}
	q.enqWg.Add(1)
	q.enqMu.Unlock()
	defer q.enqWg.Done()

	// Final check after acquiring the enqueue slot
	if atomic.LoadInt32(&q.closed) == 1 {
		return ErrQueueClosed
	}

	newOp := &types.QueueOp{
		Handler: op.Handler,
		Payload: op.Payload,
		TS:      op.TS,
	}
	if op.Extras != (types.RequestMetadata{}) {
		newOp.Extras = op.Extras
	}
	var it *types.QueueItem
	if q.wal != nil && q.walBacked {
		// WAL manages sequence - get persistent sequence from WAL
		data, err := json.Marshal(newOp)
		if err != nil {
			return err
		}
		walSeq, err := q.wal.WriteWithSequence(data)
		if err != nil {
			return err
		}
		newOp.EnqSeq = walSeq
		it = &types.QueueItem{Op: newOp, Sb: nil, Q: q}
	} else {
		// In-memory sequence (current behavior)
		newOp.EnqSeq = atomic.AddUint64(&enqSeq, 1)
		it = &types.QueueItem{Op: newOp, Sb: nil, Q: q}
	}

	select {
	case q.ch <- it:
		atomic.AddInt64(&q.inFlight, 1)
		return nil
	default:
		atomic.AddUint64(&q.dropped, 1)
		it.JobDone()
		return ErrQueueFull
	}
}

// shutdown queue
func (q *IngestQueue) Close() error {
	if atomic.LoadInt32(&q.closed) == 1 {
		return nil
	}
	atomic.StoreInt32(&q.closed, 1)
	q.closeOnce.Do(func() {
		close(q.ch)
	})
	q.enqWg.Wait()
	if q.wal != nil {
		return q.wal.Close()
	}
	return nil
}

// get output channel
func (q *IngestQueue) Out() <-chan *types.QueueItem {
	return q.ch
}

// global enqueue
func Enqueue(op *types.QueueOp) error {
	if GlobalIngestQueue != nil {
		return GlobalIngestQueue.Enqueue(op)
	}
	return ErrQueueClosed
}
