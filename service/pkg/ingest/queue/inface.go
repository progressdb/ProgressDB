package queue

import (
	"encoding/json"
	"errors"
	"sync/atomic"
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
func (q *IngestQueue) Enqueue(op *QueueOp) error {
	return q.enqueue(op)
}

// internal enqueue function
func (q *IngestQueue) enqueue(op *QueueOp) error {
	if atomic.LoadInt32(&q.closed) == 1 {
		return ErrQueueClosed
	}

	q.enqWg.Add(1)
	defer q.enqWg.Done()

	if atomic.LoadInt32(&q.closed) == 1 {
		return ErrQueueClosed
	}

	newOp := &QueueOp{
		Handler: op.Handler,
		Thread:  op.Thread,
		ID:      op.ID,
		Payload: op.Payload,
		TS:      op.TS,
	}
	if op.Extras != nil {
		newMap := make(map[string]string, len(op.Extras))
		for k, v := range op.Extras {
			newMap[k] = v
		}
		newOp.Extras = newMap
	}
	newOp.EnqSeq = atomic.AddUint64(&enqSeq, 1)

	var it *QueueItem
	if q.wal != nil && q.walBacked {
		data, err := json.Marshal(newOp)
		if err != nil {
			return err
		}
		if err := q.wal.Write(uint64(newOp.EnqSeq), data); err != nil {
			return err
		}
		it = &QueueItem{Op: newOp, Sb: nil, Q: q}
	} else {
		it = &QueueItem{Op: newOp, Sb: nil, Q: q}
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

// start single operation worker
func (q *IngestQueue) RunWorker(stop <-chan struct{}, handler func(*QueueOp) error) {
	RunWorker(q, stop, handler)
}

// start batch operation worker
func (q *IngestQueue) RunBatchWorker(stop <-chan struct{}, batchSize int, handler func([]*QueueOp) error) {
	RunBatchWorker(q, stop, batchSize, handler)
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
func (q *IngestQueue) Out() <-chan *QueueItem {
	return q.ch
}

// global enqueue
func Enqueue(op *QueueOp) error {
	if GlobalIngestQueue != nil {
		return GlobalIngestQueue.Enqueue(op)
	}
	return ErrQueueClosed
}
