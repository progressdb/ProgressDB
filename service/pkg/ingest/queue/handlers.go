package queue

import (
	"context"
	"sync/atomic"
	"time"
)

// tryenqueuebytes: copies payload into a pooled buffer and enqueues a new op using the given fields
func (q *IngestQueue) TryEnqueueBytes(handler HandlerID, thread, id string, payload []byte, ts int64) error {
	return q.TryEnqueue(&QueueOp{Handler: handler, Thread: thread, ID: id, Payload: payload, TS: ts})
}

// enqueuebytes: attempts to enqueue op, blocking until space or ctx is done; returns ctx.Err() if context expires
func (q *IngestQueue) EnqueueBytes(ctx context.Context, handler HandlerID, thread, id string, payload []byte, ts int64) error {
	return q.Enqueue(ctx, &QueueOp{Handler: handler, Thread: thread, ID: id, Payload: payload, TS: ts})
}

// enqueueop: convenience wrapper constructing an op with extras and enqueues (non-blocking); caller should pass extras map
func (q *IngestQueue) EnqueueQueueOp(handler HandlerID, thread, id string, payload []byte, ts int64, extras map[string]string) error {

	op := &QueueOp{Handler: handler, Thread: thread, ID: id, Payload: payload, TS: ts, Extras: extras}
	return q.TryEnqueue(op)
}

func (q *IngestQueue) closeInternal(drain bool) {
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
	const drainPollInterval = 10 * time.Millisecond
	ticker := time.NewTicker(drainPollInterval)
	defer ticker.Stop()
	for {
		if atomic.LoadInt64(&q.inFlight) == 0 {
			return
		}
		<-ticker.C
	}
}

// len: returns current number of items in queue
func (q *IngestQueue) Len() int { return len(q.ch) }

// cap: returns configured capacity of queue
func (q *IngestQueue) Cap() int { return q.capacity }

// dropped: returns number of ops dropped due to full queue or ctx cancellations during enqueue
func (q *IngestQueue) Dropped() uint64 { return atomic.LoadUint64(&q.dropped) }

// enqueuedtotal: returns total attempted enqueues
func (q *IngestQueue) EnqueuedTotal() uint64 { return atomic.LoadUint64(&enqueueTotal) }

// failedtotal: returns total enqueue failures
func (q *IngestQueue) FailedTotal() uint64 { return atomic.LoadUint64(&enqueueFailTotal) }
