package queue

import (
	"context"
	"sync/atomic"
	"time"
)

const drainPollInterval = 10 * time.Millisecond

// TryEnqueueBytes copies payload into a pooled buffer and enqueues a new
// Op constructed from the provided fields.
func (q *Queue) TryEnqueueBytes(handler HandlerID, thread, id string, payload []byte, ts int64) error {
	return q.TryEnqueue(&Op{Handler: handler, Thread: thread, ID: id, Payload: payload, TS: ts})
}

// Enqueue attempts to enqueue op, blocking until space is available or the
// provided context is done. Returns ctx.Err() if the context expires.
func (q *Queue) EnqueueBytes(ctx context.Context, handler HandlerID, thread, id string, payload []byte, ts int64) error {
	return q.Enqueue(ctx, &Op{Handler: handler, Thread: thread, ID: id, Payload: payload, TS: ts})
}

// EnqueueOp is a convenience wrapper that constructs an Op with Extras and
// enqueues it (non-blocking). Caller should pass header extras map.
func (q *Queue) EnqueueOp(handler HandlerID, thread, id string, payload []byte, ts int64, extras map[string]string) error {
	op := &Op{Handler: handler, Thread: thread, ID: id, Payload: payload, TS: ts, Extras: extras}
	return q.TryEnqueue(op)
}

// Close stops accepting new items, waits for in-flight items to finish processing,
// and closes the underlying channel. It is safe to call multiple times.
func (q *Queue) Close() {
	q.closeInternal(false)
}

// CloseAndDrain closes the queue channel and drains remaining items,
// ensuring their resources are released. This is primarily intended for tests.
func (q *Queue) CloseAndDrain() {
	q.closeInternal(true)
}

func (q *Queue) closeInternal(drain bool) {
	if q == nil {
		return
	}
	_ = atomic.CompareAndSwapInt32(&q.closed, 0, 1)

	// wait for any in-flight enqueuers to finish before closing the channel
	q.enqWg.Wait()

	// ensure the channel is closed exactly once
	q.closeOnce.Do(func() {
		close(q.ch)
	})

	if drain {
		for it := range q.ch {
			it.Done()
		}
		return
	}

	ticker := time.NewTicker(drainPollInterval)
	defer ticker.Stop()
	for {
		if atomic.LoadInt64(&q.inFlight) == 0 {
			return
		}
		<-ticker.C
	}
}

// Len returns the current number of items in the queue.
func (q *Queue) Len() int { return len(q.ch) }

// Cap returns the configured capacity of the queue.
func (q *Queue) Cap() int { return q.capacity }

// Dropped returns the number of operations that were dropped due to a full
// queue or context cancellations during enqueue.
func (q *Queue) Dropped() uint64 { return atomic.LoadUint64(&q.dropped) }

// EnqueuedTotal returns total attempted enqueues.
func (q *Queue) EnqueuedTotal() uint64 { return atomic.LoadUint64(&enqueueTotal) }

// FailedTotal returns total enqueue failures.
func (q *Queue) FailedTotal() uint64 { return atomic.LoadUint64(&enqueueFailTotal) }
