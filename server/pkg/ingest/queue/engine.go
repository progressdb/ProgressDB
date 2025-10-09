package queue

import (
	"context"
	"sync/atomic"

	"github.com/valyala/bytebufferpool"
)

// Queue is a bounded global in-memory queue used by the API layer to
// enqueue create/update/delete operations. It is safe for concurrent
// producers. Consumers should range over Out() to receive items.
type Queue struct {
	ch       chan *Item
	capacity int
	dropped  uint64
}

// instrumentation counters (package-local)
var (
	enqueueTotal     uint64
	enqueueFailTotal uint64
)

var enqSeq uint64

// DefaultQueue is a global default queue used by handlers. It can be
// replaced at startup by calling SetDefaultQueue.
var DefaultQueue = NewQueue(64 * 1024)

// NewQueue creates a new bounded Queue with the provided capacity. Capacity
// must be > 0.
func NewQueue(capacity int) *Queue {
	if capacity <= 0 {
		capacity = 1024
	}
	return &Queue{ch: make(chan *Item, capacity), capacity: capacity}
}

// SetDefaultQueue replaces the package default queue.
func SetDefaultQueue(q *Queue) {
	if q != nil {
		DefaultQueue = q
	}
}

// Out returns a read-only channel that consumers can range over to receive
// queued items. The returned channel is the internal channel cast to
// read-only; do not close it from callers.
func (q *Queue) Out() <-chan *Item { return q.ch }

// TryEnqueue attempts to enqueue an Op by copying payload into a pooled
// buffer. If payload is nil it enqueues an Op without buffer ownership.
// If the queue is full ErrQueueFull is returned and the caller may choose
// to spill or reject.
func (q *Queue) TryEnqueue(op *Op) error {
	atomic.AddUint64(&enqueueTotal, 1)
	// acquire an Op from the pool and copy fields
	newOp := opPool.Get().(*Op)
	*newOp = *op
	// copy Extras map shallowly to avoid sharing mutable maps
	if op.Extras != nil {
		newMap := make(map[string]string, len(op.Extras))
		for k, v := range op.Extras {
			newMap[k] = v
		}
		newOp.Extras = newMap
	}
	// assign enqueue sequence
	newOp.EnqSeq = atomic.AddUint64(&enqSeq, 1)

	var bb *bytebufferpool.ByteBuffer
	if len(op.Payload) > 0 {
		bb = bytebufferpool.Get()
		// copy payload into pooled buffer
		bb.B = append(bb.B[:0], op.Payload...)
		newOp.Payload = bb.B[:len(op.Payload)]
	}

	it := &Item{Op: newOp, buf: bb, q: q}

	select {
	case q.ch <- it:
		return nil
	default:
		// return resources
		if bb != nil {
			bytebufferpool.Put(bb)
		}
		opPool.Put(newOp)
		atomic.AddUint64(&q.dropped, 1)
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ErrQueueFull
	}
}

// Enqueue attempts to enqueue op, blocking until space is available or the
// provided context is done. Returns ctx.Err() if the context expires.
func (q *Queue) Enqueue(ctx context.Context, op *Op) error {
	atomic.AddUint64(&enqueueTotal, 1)
	// copy fields into pooled op
	newOp := opPool.Get().(*Op)
	*newOp = *op
	// assign enqueue sequence
	newOp.EnqSeq = atomic.AddUint64(&enqSeq, 1)
	if op.Extras != nil {
		newMap := make(map[string]string, len(op.Extras))
		for k, v := range op.Extras {
			newMap[k] = v
		}
		newOp.Extras = newMap
	}

	var bb *bytebufferpool.ByteBuffer
	if len(op.Payload) > 0 {
		bb = bytebufferpool.Get()
		bb.B = append(bb.B[:0], op.Payload...)
		newOp.Payload = bb.B[:len(op.Payload)]
	}
	it := &Item{Op: newOp, buf: bb, q: q}

	select {
	case q.ch <- it:
		return nil
	case <-ctx.Done():
		if bb != nil {
			bytebufferpool.Put(bb)
		}
		opPool.Put(newOp)
		atomic.AddUint64(&q.dropped, 1)
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ctx.Err()
	}
}

// RunWorker runs a worker loop that invokes handler for each dequeued Op.
// It guarantees Item.Done() is called even if handler returns an error.
// The worker exits when stop is closed or when the queue is closed.
func (q *Queue) RunWorker(stop <-chan struct{}, handler func(*Op) error) {
	for {
		select {
		case it, ok := <-q.ch:
			if !ok {
				return
			}
			func(it *Item) {
				defer it.Done()
				_ = handler(it.Op)
			}(it)
		case <-stop:
			return
		}
	}
}
