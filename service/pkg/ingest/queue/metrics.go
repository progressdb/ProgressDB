package queue

import (
	"sync/atomic"
)

// enqueuebytes: copies payload into a pooled buffer and enqueues a new op using the given fields
func (q *IngestQueue) EnqueueBytes(handler HandlerID, tid string, mid string, payload []byte, ts int64) error {
	return q.Enqueue(&QueueOp{Handler: handler, TID: tid, MID: mid, Payload: payload, TS: ts})
}

// len: returns current number of items in queue
func (q *IngestQueue) Len() int { return len(q.ch) }

// cap: returns configured capacity of queue
func (q *IngestQueue) Cap() int { return q.capacity }

// dropped: returns number of ops dropped due to full queue or ctx cancellations during enqueue
func (q *IngestQueue) Dropped() uint64 { return atomic.LoadUint64(&q.dropped) }
