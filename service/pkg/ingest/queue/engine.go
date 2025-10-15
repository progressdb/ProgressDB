package queue

import (
	"container/heap"
	"context"
	"math"
	"sync/atomic"

	"github.com/valyala/bytebufferpool"
	"progressdb/pkg/telemetry"
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
	tr := telemetry.Track("ingest.queue_try_enqueue")
	defer tr.Finish()

	// increment total enqueues attempted
	atomic.AddUint64(&enqueueTotal, 1)

	// check if queue is closed
	if atomic.LoadInt32(&q.closed) == 1 {
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ErrQueueClosed
	}

	// mark this enqueue in the wait group
	q.enqWg.Add(1)
	defer q.enqWg.Done()

	// bouble-check if queue is closed
	if atomic.LoadInt32(&q.closed) == 1 {
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ErrQueueClosed
	}

	// clone operation from pool
	newOp := queueOpPool.Get().(*QueueOp)
	*newOp = *op
	// copy extras map if present
	if op.Extras != nil {
		newMap := make(map[string]string, len(op.Extras))
		for k, v := range op.Extras {
			newMap[k] = v
		}
		newOp.Extras = newMap
	}
	// assign unique sequence
	newOp.EnqSeq = atomic.AddUint64(&enqSeq, 1)
	tr.Mark("clone_op")

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
			tr.Mark("serialize")

			// append to wal
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
			// track outstanding operations
			q.ackMu.Lock()
			if q.outstanding == nil {
				q.outstanding = make(map[int64]struct{})
			}
			q.outstanding[offset] = struct{}{}
			heap.Push(&q.outstandingH, offset)
			q.ackMu.Unlock()
			tr.Mark("wal_append")

			// send to channel or drop
			it := &QueueItem{Op: newOp, Sb: sb, Buf: bb, Q: q}
			select {
			case q.ch <- it:
				atomic.AddInt64(&q.inFlight, 1)
				tr.Mark("enqueue_channel")
				return nil
			default:
				sb.release()
				queueOpPool.Put(newOp)
				atomic.AddUint64(&q.dropped, 1)
				atomic.AddUint64(&enqueueFailTotal, 1)
				return ErrQueueFull
			}
		}

		// serialize operation
		data, err := serializeOp(newOp)
		if err != nil {
			queueOpPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		tr.Mark("serialize")
		// append based on mode
		var offset int64
		if q.walMode == WalModeSync {
			offset, err = q.wal.AppendSync(data)
		} else {
			// non-blocking append
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
		// track outstanding
		q.ackMu.Lock()
		if q.outstanding == nil {
			q.outstanding = make(map[int64]struct{})
		}
		q.outstanding[offset] = struct{}{}
		heap.Push(&q.outstandingH, offset)
		q.ackMu.Unlock()
		tr.Mark("wal_append")
	}

	// fallback to in-memory enqueue
	err := q.tryEnqueueInMemory(newOp)
	tr.Mark("enqueue_memory")
	return err
}

// blocking enqueue with context
func (q *IngestQueue) Enqueue(ctx context.Context, op *QueueOp) error {
	tr := telemetry.Track("ingest.queue_enqueue")
	defer tr.Finish()

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

	tr.Mark("clone_op")
	// clone from pool
	newOp := queueOpPool.Get().(*QueueOp)
	*newOp = *op
	newOp.EnqSeq = atomic.AddUint64(&enqSeq, 1)
	// copy extras
	if op.Extras != nil {
		newMap := make(map[string]string, len(op.Extras))
		for k, v := range op.Extras {
			newMap[k] = v
		}
		newOp.Extras = newMap
	}

	// wal-backed with context
	if q != nil && q.wal != nil {
		if q.walBacked {
			tr.Mark("serialize")
			// serialize to buffer
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
			tr.Mark("wal_append")
			// append with context
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
			// track outstanding
			q.ackMu.Lock()
			if q.outstanding == nil {
				q.outstanding = make(map[int64]struct{})
			}
			q.outstanding[offset] = struct{}{}
			heap.Push(&q.outstandingH, offset)
			q.ackMu.Unlock()

			tr.Mark("enqueue_channel")
			// send or cancel
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

		tr.Mark("serialize")
		// serialize
		data, err := serializeOp(newOp)
		if err != nil {
			queueOpPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		tr.Mark("wal_append")
		// append with context
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
		// track outstanding
		q.ackMu.Lock()
		if q.outstanding == nil {
			q.outstanding = make(map[int64]struct{})
		}
		q.outstanding[offset] = struct{}{}
		heap.Push(&q.outstandingH, offset)
		q.ackMu.Unlock()
	}

	tr.Mark("enqueue_memory")
	// in-memory enqueue
	return q.enqueueInMemory(ctx, newOp)
}

// start single operation worker
func (q *IngestQueue) RunWorker(stop <-chan struct{}, handler func(*QueueOp) error) {
	RunWorker(q, stop, handler)
}

// acknowledge processed offset
func (q *IngestQueue) ack(offset int64) {
	tr := telemetry.Track("ingest.queue_ack")
	defer tr.Finish()

	if q == nil || q.wal == nil {
		return
	}
	q.ackMu.Lock()
	delete(q.outstanding, offset)
	tr.Mark("update_outstanding")

	// clean heap of processed offsets
	for q.outstandingH.Len() > 0 {
		top := q.outstandingH[0]
		if _, ok := q.outstanding[top]; ok {
			break
		}
		heap.Pop(&q.outstandingH)
	}
	tr.Mark("clean_heap")

	// calculate new minimum
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
		tr.Mark("wal_truncate")
	}
}

// perform wal truncation
func (q *IngestQueue) doTruncate() {
	if q == nil || q.wal == nil {
		return
	}
	q.ackMu.Lock()
	// clean processed offsets
	for q.outstandingH.Len() > 0 {
		top := q.outstandingH[0]
		if _, ok := q.outstanding[top]; ok {
			break
		}
		heap.Pop(&q.outstandingH)
	}
	// find new minimum
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

// non-blocking in-memory enqueue
func (q *IngestQueue) tryEnqueueInMemory(newOp *QueueOp) error {
	// handle payload buffer
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

// blocking in-memory enqueue
func (q *IngestQueue) enqueueInMemory(ctx context.Context, newOp *QueueOp) error {
	// prepare buffer
	var bb *bytebufferpool.ByteBuffer
	if len(newOp.Payload) > 0 {
		bb = bytebufferpool.Get()
		bb.B = append(bb.B[:0], newOp.Payload...)
		newOp.Payload = bb.B[:len(newOp.Payload)]
	}

	it := queueItemPool.Get().(*QueueItem)
	it.Op = newOp
	it.Buf = bb
	it.Q = q

	select {
	case q.ch <- it:
		atomic.AddInt64(&q.inFlight, 1)
		return nil
	case <-ctx.Done():
		if bb != nil {
			bytebufferpool.Put(bb)
		}
		queueOpPool.Put(newOp)
		queueItemPool.Put(it)
		atomic.AddUint64(&q.dropped, 1)
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ctx.Err()
	}
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
