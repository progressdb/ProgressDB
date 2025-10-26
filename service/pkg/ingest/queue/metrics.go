package queue

import (
	"sync/atomic"
)

func (q *IngestQueue) EnqueueBytes(handler HandlerID, tid string, mid string, payload []byte, ts int64) error {
	return q.Enqueue(&QueueOp{Handler: handler, Payload: payload, TS: ts})
}
func (q *IngestQueue) Len() int        { return len(q.ch) }
func (q *IngestQueue) Cap() int        { return q.capacity }
func (q *IngestQueue) Dropped() uint64 { return atomic.LoadUint64(&q.dropped) }
