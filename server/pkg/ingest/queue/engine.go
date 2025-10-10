package queue

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/valyala/bytebufferpool"
)

// TODO: queue full - drop strategy
// TODO: handler error - handlng strategy

// Default and configuration values.
const defaultQueueCapacity = 64 * 1024
const fallbackQueueCapacity = 1024
const defaultBatchSize = 256

// Counters for instrumentation.
var (
	enqueueTotal     uint64
	enqueueFailTotal uint64
	enqSeq           uint64
)

// DefaultQueue is the global queue for handlers; can be overridden.
var DefaultQueue = NewQueue(defaultQueueCapacity)

// Queue is a threadsafe, fixed-size in-memory queue of Op items.
type Queue struct {
	ch       chan *Item
	capacity int
	dropped  uint64
	closed   int32

	enqWg     sync.WaitGroup
	closeOnce sync.Once
	inFlight  int64
}

// NewQueue creates a bounded Queue of given capacity (>0).
func NewQueue(capacity int) *Queue {
	if capacity <= 0 {
		capacity = fallbackQueueCapacity
	}
	return &Queue{ch: make(chan *Item, capacity), capacity: capacity}
}

// SetDefaultQueue sets the package default if q is non-nil.
func SetDefaultQueue(q *Queue) {
	if q != nil {
		DefaultQueue = q
	}
}

// Out exposes Items for consumers (do not close).
func (q *Queue) Out() <-chan *Item { return q.ch }

// TryEnqueue enqueues an Op without blocking; returns ErrQueueFull if full.
func (q *Queue) TryEnqueue(op *Op) error {
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

	newOp := opPool.Get().(*Op)
	*newOp = *op
	// Shallow copy Extras map if present.
	if op.Extras != nil {
		newMap := make(map[string]string, len(op.Extras))
		for k, v := range op.Extras {
			newMap[k] = v
		}
		newOp.Extras = newMap
	}
	newOp.EnqSeq = atomic.AddUint64(&enqSeq, 1)

	var bb *bytebufferpool.ByteBuffer
	if len(op.Payload) > 0 {
		bb = bytebufferpool.Get()
		bb.B = append(bb.B[:0], op.Payload...)
		newOp.Payload = bb.B[:len(op.Payload)]
	}

	it := &Item{Op: newOp, buf: bb, q: q}

	select {
	case q.ch <- it:
		atomic.AddInt64(&q.inFlight, 1)
		return nil
	default:
		// Clean up pooled resources on failure.
		if bb != nil {
			bytebufferpool.Put(bb)
		}
		opPool.Put(newOp)
		atomic.AddUint64(&q.dropped, 1)
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ErrQueueFull
	}
}

// Enqueue blocks until op is enqueued or ctx is cancelled.
func (q *Queue) Enqueue(ctx context.Context, op *Op) error {
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

	newOp := opPool.Get().(*Op)
	*newOp = *op
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
		atomic.AddInt64(&q.inFlight, 1)
		return nil
	case <-ctx.Done():
		// Clean up on cancellation.
		if bb != nil {
			bytebufferpool.Put(bb)
		}
		opPool.Put(newOp)
		atomic.AddUint64(&q.dropped, 1)
		atomic.AddUint64(&enqueueFailTotal, 1)
		return ctx.Err()
	}
}

// RunWorker dequeues items and calls handler for each, calling Item.Done() always.
// Exits when stop or the queue closes.
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

// RunBatchWorker drains up to batchSize items from the queue and invokes the handler once per batch.
func (q *Queue) RunBatchWorker(stop <-chan struct{}, batchSize int, handler func([]*Op) error) {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	for {
		select {
		case <-stop:
			return
		default:
		}

		var items []*Item

		// Block until the first item is available or stop is signaled.
		select {
		case it, ok := <-q.ch:
			if !ok {
				return
			}
			items = append(items, it)
		case <-stop:
			return
		}

	collect:
		for len(items) < batchSize {
			select {
			case it, ok := <-q.ch:
				if !ok {
					break collect
				}
				items = append(items, it)
			default:
				break collect
			}
		}

		func(batch []*Item) {
			defer func() {
				for _, it := range batch {
					it.Done()
				}
			}()
			ops := make([]*Op, len(batch))
			for i, it := range batch {
				ops[i] = it.Op
			}
			_ = handler(ops)
		}(items)
	}
}
