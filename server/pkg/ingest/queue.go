package ingest

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/valyala/bytebufferpool"
)

// OpType represents an operation kind for the ingest pipeline.
type OpType string

const (
	OpCreate OpType = "create"
	OpUpdate OpType = "update"
	OpDelete OpType = "delete"
)

// Op is a lightweight in-memory representation of a create/update/delete
// operation destined for the persistence pipeline. Payload may be backed by
// a pooled ByteBuffer; consumers must call Item.Done() when finished.
type Op struct {
	Type   OpType
	Thread string
	ID     string
	// Payload holds the raw bytes for the operation (may be nil).
	Payload []byte
	// TS is an optional client/server timestamp (nanoseconds).
	TS int64
	// EnqSeq is a monotonic enqueue sequence assigned when the op is
	// accepted into the in-memory queue. It is used for deterministic
	// ordering inside batches.
	EnqSeq uint64
	// Extras holds small metadata extracted from HTTP request headers
	// (e.g. role, identity, request id). It is optional.
	Extras map[string]string
}

var (
	// ErrQueueFull is returned by TryEnqueue when the queue is at capacity.
	ErrQueueFull = errors.New("ingest queue full")
)

// Item wraps an Op and owns a pooled ByteBuffer if one was used. Consumers
// MUST call Done() exactly once after processing the item to return
// pooled resources.
type Item struct {
	Op *Op

	// internal fields
	buf  *bytebufferpool.ByteBuffer
	once sync.Once
	q    *Queue
}

// Done releases internal pooled resources (buffer + op) back to the pool.
func (it *Item) Done() {
	it.once.Do(func() {
		if it.buf != nil {
			// avoid retaining huge buffers in the pool
			if cap(it.buf.B) > maxPooledBuffer {
				// drop the buffer so GC can reclaim the underlying array
				it.buf = nil
			} else {
				bytebufferpool.Put(it.buf)
				it.buf = nil
			}
		}
		// clear slice header to avoid retention
		if it.Op != nil {
			it.Op.Payload = nil
			it.Op.Extras = nil
			opPool.Put(it.Op)
			it.Op = nil
		}
		// return Item to pool
		itemPool.Put(it)
	})
}

// Queue is a bounded global in-memory queue used by the API layer to
// enqueue create/update/delete operations. It is safe for concurrent
// producers. Consumers should range over Out() to receive items.
type Queue struct {
	ch       chan *Item
	capacity int
	dropped  uint64
}

var opPool = sync.Pool{New: func() any { return &Op{} }}
var itemPool = sync.Pool{New: func() any { return &Item{} }}

// instrumentation counters (package-local)
var (
	enqueueTotal     uint64
	enqueueFailTotal uint64
)

// maxPooledBuffer controls the largest buffer size that will be returned
// to the pooled ByteBuffer. Buffers larger than this will be dropped to
// avoid unbounded resident memory.
var maxPooledBuffer = 256 * 1024 // 256 KiB

// NewQueue creates a new bounded Queue with the provided capacity. Capacity
// must be > 0.
func NewQueue(capacity int) *Queue {
	if capacity <= 0 {
		capacity = 1024
	}
	return &Queue{ch: make(chan *Item, capacity), capacity: capacity}
}

var enqSeq uint64

// DefaultQueue is a global default queue used by handlers. It can be
// replaced at startup by calling SetDefaultQueue.
var DefaultQueue = NewQueue(64 * 1024)

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

// TryEnqueueBytes copies payload into a pooled buffer and enqueues a new
// Op constructed from the provided fields.
func (q *Queue) TryEnqueueBytes(typ OpType, thread, id string, payload []byte, ts int64) error {
	return q.TryEnqueue(&Op{Type: typ, Thread: thread, ID: id, Payload: payload, TS: ts})
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

// EnqueueBytes copies payload into a pooled buffer and enqueues a new Op constructed from the provided fields.
func (q *Queue) EnqueueBytes(ctx context.Context, typ OpType, thread, id string, payload []byte, ts int64) error {
	return q.Enqueue(ctx, &Op{Type: typ, Thread: thread, ID: id, Payload: payload, TS: ts})
}

// CloseAndDrain closes the queue channel and drains remaining items,
// ensuring their resources are released.
func (q *Queue) CloseAndDrain() {
	close(q.ch)
	for it := range q.ch {
		it.Done()
	}
}

// EnqueueOp is a convenience wrapper that constructs an Op with Extras and
// enqueues it (non-blocking). Caller should pass header extras map.
func (q *Queue) EnqueueOp(typ OpType, thread, id string, payload []byte, ts int64, extras map[string]string) error {
	op := &Op{Type: typ, Thread: thread, ID: id, Payload: payload, TS: ts, Extras: extras}
	return q.TryEnqueue(op)
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

// Len returns the current number of items in the queue.
func (q *Queue) Len() int { return len(q.ch) }

// Cap returns the configured capacity of the queue.
func (q *Queue) Cap() int { return q.capacity }

// Dropped returns the number of operations that were dropped due to a full
// queue or context cancellations during enqueue.
func (q *Queue) Dropped() uint64 { return atomic.LoadUint64(&q.dropped) }
