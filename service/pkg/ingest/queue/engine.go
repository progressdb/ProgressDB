package queue

import (
	"context"
	"sync/atomic"

	"github.com/valyala/bytebufferpool"
)

// TODO: queue full - drop strategy
// TODO: handler error - handlng strategy

var (
	// total enqueues attempted
	enqueueTotal uint64
	// total failed enqueues
	enqueueFailTotal uint64
	// enqueue sequence number
	enqSeq uint64
)

// attempt enqueue without blocking
func (q *IngestQueue) TryEnqueue(op *QueueOp) error {
	atomic.AddUint64(&enqueueTotal, 1)

	if atomic.LoadInt32(&q.closed) == 1 {
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ErrQueueClosed
	}

	q.enqWg.Add(1)
	defer q.enqWg.Done()

	if atomic.LoadInt32(&q.closed) == 1 {
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ErrQueueClosed
	}

	newOp := queueOpPool.Get().(*QueueOp)
	*newOp = *op
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
		bb := bytebufferpool.Get()
		payloadSlice, err := serializeOpToBB(newOp, bb)
		if err != nil {
			bytebufferpool.Put(bb)
			queueOpPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		newOp.Payload = payloadSlice
		if err := q.wal.Write(uint64(newOp.EnqSeq), payloadSlice); err != nil {
			bytebufferpool.Put(bb)
			queueOpPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		it = &QueueItem{Op: newOp, Sb: nil, Buf: bb, Q: q}
	} else {
		it = &QueueItem{Op: newOp, Sb: nil, Buf: nil, Q: q}
	}

	select {
	case q.ch <- it:
		atomic.AddInt64(&q.inFlight, 1)
		return nil
	default:
		atomic.AddUint64(&enqueueFailTotal, 1)
		atomic.AddUint64(&q.dropped, 1)
		it.Done()
		return ErrQueueFull
	}
}

// blocking enqueue with context
func (q *IngestQueue) Enqueue(ctx context.Context, op *QueueOp) error {
	atomic.AddUint64(&enqueueTotal, 1)

	if atomic.LoadInt32(&q.closed) == 1 {
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ErrQueueClosed
	}

	q.enqWg.Add(1)
	defer q.enqWg.Done()

	if atomic.LoadInt32(&q.closed) == 1 {
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ErrQueueClosed
	}

	newOp := queueOpPool.Get().(*QueueOp)
	*newOp = *op
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
		bb := bytebufferpool.Get()
		payloadSlice, err := serializeOpToBB(newOp, bb)
		if err != nil {
			bytebufferpool.Put(bb)
			queueOpPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		newOp.Payload = payloadSlice
		if err := q.wal.Write(uint64(newOp.EnqSeq), payloadSlice); err != nil {
			bytebufferpool.Put(bb)
			queueOpPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		it = &QueueItem{Op: newOp, Sb: nil, Buf: bb, Q: q}
	} else {
		it = &QueueItem{Op: newOp, Sb: nil, Buf: nil, Q: q}
	}

	select {
	case q.ch <- it:
		atomic.AddInt64(&q.inFlight, 1)
		return nil
	case <-ctx.Done():
		it.Done()
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ctx.Err()
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

// global non-blocking enqueue
func TryEnqueue(op *QueueOp) error {
	if DefaultIngestQueue != nil {
		return DefaultIngestQueue.TryEnqueue(op)
	}
	return ErrQueueClosed
}

// global blocking enqueue
func Enqueue(ctx context.Context, op *QueueOp) error {
	if DefaultIngestQueue != nil {
		return DefaultIngestQueue.Enqueue(ctx, op)
	}
	return ErrQueueClosed
}
