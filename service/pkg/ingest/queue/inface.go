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
	enqSeq uint64
)

var (
	ErrQueueFull   = errors.New("ingest queue full")
	ErrQueueClosed = errors.New("ingest queue closed")
)

func (q *IngestQueue) Enqueue(op *types.QueueOp) error {
	return q.enqueue(op)
}

func (q *IngestQueue) EnqueueReplay(op *types.QueueOp) error {
	return q.enqueueReplay(op)
}

func (q *IngestQueue) enqueue(op *types.QueueOp) error {
	q.enqMu.Lock()
	if atomic.LoadInt32(&q.closed) == 1 {
		q.enqMu.Unlock()
		return ErrQueueClosed
	}
	q.enqWg.Add(1)
	q.enqMu.Unlock()
	defer q.enqWg.Done()

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

func (q *IngestQueue) enqueueReplay(op *types.QueueOp) error {
	q.enqMu.Lock()
	if atomic.LoadInt32(&q.closed) == 1 {
		q.enqMu.Unlock()
		return ErrQueueClosed
	}
	q.enqWg.Add(1)
	q.enqMu.Unlock()
	defer q.enqWg.Done()

	if atomic.LoadInt32(&q.closed) == 1 {
		return ErrQueueClosed
	}

	newOp := &types.QueueOp{
		Handler: op.Handler,
		Payload: op.Payload,
		TS:      op.TS,
		EnqSeq:  op.EnqSeq,
	}
	if op.Extras != (types.RequestMetadata{}) {
		newOp.Extras = op.Extras
	}

	it := &types.QueueItem{Op: newOp, Sb: nil, Q: q}

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

func (q *IngestQueue) Out() <-chan *types.QueueItem {
	return q.ch
}

func Enqueue(op *types.QueueOp) error {
	if GlobalIngestQueue != nil {
		return GlobalIngestQueue.Enqueue(op)
	}
	return ErrQueueClosed
}
