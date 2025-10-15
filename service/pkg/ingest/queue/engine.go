package queue

import (
	"container/heap"
	"context"
	"math"
	"sync/atomic"

	"github.com/valyala/bytebufferpool"
)

// TODO: queue full - drop strategy
// TODO: handler error - handlng strategy

var (
	enqueueTotal     uint64
	enqueueFailTotal uint64
	enqSeq           uint64
	defaultBackend   QueueBackend
)

// Core types relocated to types.go

// TryEnqueue enqueues an Op without blocking; returns ErrQueueFull if full.
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

	if q != nil && q.wal != nil {
		if q.walBacked {
			bb := bytebufferpool.Get()
			payloadSlice, err := serializeOpToBB(newOp, bb)
			if err != nil {
				bytebufferpool.Put(bb)
				queueOpPool.Put(newOp)
				atomic.AddUint64(&enqueueFailTotal, 1)
				return err
			}
			newOp.Payload = payloadSlice
			sb := newSharedBuf(bb, 2)
			// Use the non-context pooled-append in the non-blocking path.
			// `TryEnqueue` does not have a caller-provided context, so
			// we must use the non-context variants to avoid referencing
			// an undefined `ctx` variable.
			var offset int64
			var walErr error
			offset, walErr = q.wal.AppendPooled(sb)
			if walErr != nil {
				sb.release()
				queueOpPool.Put(newOp)
				atomic.AddUint64(&enqueueFailTotal, 1)
				return walErr
			}
			newOp.WalOffset = offset
			q.ackMu.Lock()
			if q.outstanding == nil {
				q.outstanding = make(map[int64]struct{})
			}
			q.outstanding[offset] = struct{}{}
			heap.Push(&q.outstandingH, offset)
			q.ackMu.Unlock()

			it := &QueueItem{Op: newOp, Sb: sb, Buf: bb, Q: q}
			select {
			case q.ch <- it:
				atomic.AddInt64(&q.inFlight, 1)
				return nil
			default:
				sb.release()
				queueOpPool.Put(newOp)
				atomic.AddUint64(&q.dropped, 1)
				atomic.AddUint64(&enqueueFailTotal, 1)
				return ErrQueueFull
			}
		}

		data, err := serializeOp(newOp)
		if err != nil {
			queueOpPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		var offset int64
		if q.walMode == WalModeSync {
			offset, err = q.wal.AppendSync(data)
		} else {
			// Non-blocking path: use Append (non-context) variant.
			var walErr error
			offset, walErr = q.wal.Append(data)
			if walErr != nil {
				queueOpPool.Put(newOp)
				atomic.AddUint64(&enqueueFailTotal, 1)
				return walErr
			}
		}
		if err != nil {
			queueOpPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		newOp.WalOffset = offset
		q.ackMu.Lock()
		if q.outstanding == nil {
			q.outstanding = make(map[int64]struct{})
		}
		q.outstanding[offset] = struct{}{}
		heap.Push(&q.outstandingH, offset)
		q.ackMu.Unlock()
	}

	// Non-WAL (in-memory) path handled by memory.go helper.
	return q.tryEnqueueInMemory(newOp)
}

// Enqueue blocks until op is enqueued or ctx is cancelled.
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
	newOp.EnqSeq = atomic.AddUint64(&enqSeq, 1)
	if op.Extras != nil {
		newMap := make(map[string]string, len(op.Extras))
		for k, v := range op.Extras {
			newMap[k] = v
		}
		newOp.Extras = newMap
	}

	if q != nil && q.wal != nil {
		if q.walBacked {
			bb := bytebufferpool.Get()
			payloadSlice, err := serializeOpToBB(newOp, bb)
			if err != nil {
				bytebufferpool.Put(bb)
				queueOpPool.Put(newOp)
				atomic.AddUint64(&enqueueFailTotal, 1)
				return err
			}
			newOp.Payload = payloadSlice
			sb := newSharedBuf(bb, 2)
			var offset int64
			var walErr error
			if q.walMode == WalModeSync {
				offset, walErr = q.wal.AppendPooledCtx(sb, ctx)
			} else {
				offset, walErr = q.wal.AppendPooledCtx(sb, ctx)
			}
			if walErr != nil {
				sb.release()
				queueOpPool.Put(newOp)
				atomic.AddUint64(&enqueueFailTotal, 1)
				return walErr
			}
			newOp.WalOffset = offset
			q.ackMu.Lock()
			if q.outstanding == nil {
				q.outstanding = make(map[int64]struct{})
			}
			q.outstanding[offset] = struct{}{}
			heap.Push(&q.outstandingH, offset)
			q.ackMu.Unlock()

			it := &QueueItem{Op: newOp, Sb: sb, Buf: bb, Q: q}
			select {
			case q.ch <- it:
				atomic.AddInt64(&q.inFlight, 1)
				return nil
			case <-ctx.Done():
				sb.release()
				queueOpPool.Put(newOp)
				atomic.AddUint64(&q.dropped, 1)
				atomic.AddUint64(&enqueueFailTotal, 1)
				return ctx.Err()
			}
		}

		data, err := serializeOp(newOp)
		if err != nil {
			queueOpPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		var offset int64
		if q.walMode == WalModeSync {
			offset, err = q.wal.AppendSync(data)
		} else {
			offset, err = q.wal.AppendCtx(data, ctx)
		}
		if err != nil {
			queueOpPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		newOp.WalOffset = offset
		q.ackMu.Lock()
		if q.outstanding == nil {
			q.outstanding = make(map[int64]struct{})
		}
		q.outstanding[offset] = struct{}{}
		heap.Push(&q.outstandingH, offset)
		q.ackMu.Unlock()
	}

	// Non-WAL (in-memory) path handled by memory.go helper.
	return q.enqueueInMemory(ctx, newOp)
}

func (q *IngestQueue) RunWorker(stop <-chan struct{}, handler func(*QueueOp) error) {
	RunWorker(q, stop, handler)
}

func (q *IngestQueue) ack(offset int64) {
	if q == nil || q.wal == nil {
		return
	}
	q.ackMu.Lock()
	delete(q.outstanding, offset)

	for q.outstandingH.Len() > 0 {
		top := q.outstandingH[0]
		if _, ok := q.outstanding[top]; ok {
			break
		}
		heap.Pop(&q.outstandingH)
	}

	var newMin int64
	if q.outstandingH.Len() == 0 {
		newMin = math.MaxInt64
	} else {
		newMin = q.outstandingH[0]
	}
	shouldTruncate := q.lastTruncated == -1 || newMin > q.lastTruncated
	if shouldTruncate {
		q.lastTruncated = newMin
	}
	q.ackMu.Unlock()

	if shouldTruncate {
		_ = q.wal.TruncateBefore(newMin)
	}
}

func (q *IngestQueue) doTruncate() {
	if q == nil || q.wal == nil {
		return
	}
	q.ackMu.Lock()
	for q.outstandingH.Len() > 0 {
		top := q.outstandingH[0]
		if _, ok := q.outstanding[top]; ok {
			break
		}
		heap.Pop(&q.outstandingH)
	}
	var newMin int64
	if q.outstandingH.Len() == 0 {
		newMin = math.MaxInt64
	} else {
		newMin = q.outstandingH[0]
	}
	shouldTruncate := q.lastTruncated == -1 || newMin > q.lastTruncated
	if shouldTruncate {
		q.lastTruncated = newMin
	}
	q.ackMu.Unlock()

	if shouldTruncate {
		_ = q.wal.TruncateBefore(newMin)
	}
}

func (q *IngestQueue) RunBatchWorker(stop <-chan struct{}, batchSize int, handler func([]*QueueOp) error) {
	RunBatchWorker(q, stop, batchSize, handler)
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

func (q *IngestQueue) Out() <-chan *QueueItem {
	return q.ch
}

func (q *IngestQueue) tryEnqueueInMemory(newOp *QueueOp) error {
	var bb *bytebufferpool.ByteBuffer
	if len(newOp.Payload) > 0 {
		bb = bytebufferpool.Get()
		bb.B = append(bb.B[:0], newOp.Payload...)
		newOp.Payload = bb.B[:len(newOp.Payload)]
	}

	it := &QueueItem{Op: newOp, Buf: bb, Q: q}

	select {
	case q.ch <- it:
		atomic.AddInt64(&q.inFlight, 1)
		return nil
	default:
		if bb != nil {
			bytebufferpool.Put(bb)
		}
		queueOpPool.Put(newOp)
		atomic.AddUint64(&q.dropped, 1)
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ErrQueueFull
	}
}

func (q *IngestQueue) enqueueInMemory(ctx context.Context, newOp *QueueOp) error {
	var bb *bytebufferpool.ByteBuffer
	if len(newOp.Payload) > 0 {
		bb = bytebufferpool.Get()
		bb.B = append(bb.B[:0], newOp.Payload...)
		newOp.Payload = bb.B[:len(newOp.Payload)]
	}

	it := &QueueItem{Op: newOp, Buf: bb, Q: q}

	select {
	case q.ch <- it:
		atomic.AddInt64(&q.inFlight, 1)
		return nil
	case <-ctx.Done():
		if bb != nil {
			bytebufferpool.Put(bb)
		}
		queueOpPool.Put(newOp)
		atomic.AddUint64(&q.dropped, 1)
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ctx.Err()
	}
}

// Package-level dispatcher: prefer the configured backend if present,
// otherwise fall back to the legacy DefaultIngestQueue pointer.
func TryEnqueue(op *QueueOp) error {
	if defaultBackend != nil {
		return defaultBackend.TryEnqueue(op)
	}
	if DefaultIngestQueue != nil {
		return DefaultIngestQueue.TryEnqueue(op)
	}
	return ErrQueueClosed
}

func Enqueue(ctx context.Context, op *QueueOp) error {
	if defaultBackend != nil {
		return defaultBackend.Enqueue(ctx, op)
	}
	if DefaultIngestQueue != nil {
		return DefaultIngestQueue.Enqueue(ctx, op)
	}
	return ErrQueueClosed
}
