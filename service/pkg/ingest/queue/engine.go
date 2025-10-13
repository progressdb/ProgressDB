package queue

import (
	"container/heap"
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/bytebufferpool"

	"progressdb/pkg/config"
)

// TODO: queue full - drop strategy
// TODO: handler error - handlng strategy

// Note: canonical defaults live in `service/pkg/config` and the runtime
// startup path should construct queues from that config. This package does
// not provide configuration defaults — callers must supply validated
// values. If callers pass invalid values, this is a programming error.

// Counters for instrumentation.
var (
	enqueueTotal     uint64
	enqueueFailTotal uint64
	enqSeq           uint64
)

// DefaultQueue is the global queue for handlers; can be overridden.
// It is intentionally nil until startup constructs and registers the
// canonical queue (via SetDefaultQueue). Tests may construct local queues
// directly with NewQueue for isolation.
var DefaultQueue *Queue

// Queue is a threadsafe, fixed-size in-memory queue of Op items.
type Queue struct {
	ch       chan *Item
	capacity int
	dropped  uint64
	closed   int32

	enqWg     sync.WaitGroup
	closeOnce sync.Once
	inFlight  int64

	// WAL related
	wal     WAL
	walMode int // 0=none,1=batch,2=sync

	// ack tracking for truncation
	ackMu         sync.Mutex
	outstanding   map[int64]struct{}
	outstandingH  offsetHeap
	lastTruncated int64
}

// NewQueue creates a bounded Queue of given capacity (>0).
func NewQueue(capacity int) *Queue {
	if capacity <= 0 {
		panic("queue.NewQueue: capacity must be > 0; ensure config.ValidateConfig() applied defaults")
	}
	return &Queue{ch: make(chan *Item, capacity), capacity: capacity}
}

// NewQueueFromConfig constructs a Queue from a typed `config.QueueConfig`.
// Callers should ensure `config.ValidateConfig()` was run so fields are populated.
func NewQueueFromConfig(qc config.QueueConfig) *Queue {
	return NewQueue(qc.Capacity)
}

// WAL modes
const (
	WalModeNone = iota
	WalModeBatch
	WalModeSync
)

// QueueOptions specify construction options for the queue including WAL.
type QueueOptions struct {
	Capacity int
	WAL      WAL
	// Mode: "none", "batch", "sync". If WAL is nil this is ignored.
	Mode string
	// Recover controls whether to replay WAL entries into the in-memory queue on startup.
	Recover bool
	// TruncateInterval controls optional background truncation of WAL files.
	// If zero, no background truncation is started.
	TruncateInterval time.Duration
}

// NewQueueWithOptions creates a Queue with options (WAL, capacity, recovery).
func NewQueueWithOptions(opts *QueueOptions) *Queue {
	if opts == nil || opts.Capacity <= 0 {
		panic("queue.NewQueueWithOptions: opts and opts.Capacity must be provided; ensure config.ValidateConfig() applied defaults")
	}
	cap := opts.Capacity
	q := &Queue{ch: make(chan *Item, cap), capacity: cap, outstanding: make(map[int64]struct{}), outstandingH: offsetHeap{}, lastTruncated: -1}
	if opts != nil && opts.WAL != nil {
		q.wal = opts.WAL
		switch opts.Mode {
		case "sync":
			q.walMode = WalModeSync
		case "batch":
			q.walMode = WalModeBatch
		default:
			q.walMode = WalModeBatch
		}

		if opts.Recover {
			// stream WAL records into the queue (do not re-append)
			_ = q.wal.RecoverStream(func(r WALRecord) error {
				op, err := deserializeOp(r.Data)
				if err != nil {
					// skip malformed records
					return nil
				}
				op.WalOffset = r.Offset
				// mark outstanding
				q.ackMu.Lock()
				if q.outstanding == nil {
					q.outstanding = make(map[int64]struct{})
				}
				q.outstanding[r.Offset] = struct{}{}
				heap.Push(&q.outstandingH, r.Offset)
				q.ackMu.Unlock()
				// push into channel non-blocking (assume capacity sufficient on startup)
				it := &Item{Op: op, buf: nil, q: q}
				q.ch <- it
				atomic.AddInt64(&q.inFlight, 1)
				return nil
			})
		}
	}
	// start background truncation if requested
	if opts != nil && opts.TruncateInterval > 0 && q.wal != nil {
		go func() {
			ticker := time.NewTicker(opts.TruncateInterval)
			defer ticker.Stop()
			for range ticker.C {
				q.doTruncate()
			}
		}()
	}
	return q
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

	// If WAL configured, persist before making item visible.
	if q != nil && q.wal != nil {
		data, err := serializeOp(newOp)
		if err != nil {
			opPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		var offset int64
		if q.walMode == WalModeSync {
			offset, err = q.wal.AppendSync(data)
		} else {
			offset, err = q.wal.Append(data)
		}
		if err != nil {
			opPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		newOp.WalOffset = offset
		// mark outstanding (set + heap)
		q.ackMu.Lock()
		if q.outstanding == nil {
			q.outstanding = make(map[int64]struct{})
		}
		q.outstanding[offset] = struct{}{}
		heap.Push(&q.outstandingH, offset)
		q.ackMu.Unlock()
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

	// If WAL configured, persist before making item visible.
	if q != nil && q.wal != nil {
		data, err := serializeOp(newOp)
		if err != nil {
			opPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		var offset int64
		if q.walMode == WalModeSync {
			offset, err = q.wal.AppendSync(data)
		} else {
			offset, err = q.wal.Append(data)
		}
		if err != nil {
			opPool.Put(newOp)
			atomic.AddUint64(&enqueueFailTotal, 1)
			return err
		}
		newOp.WalOffset = offset
		// mark outstanding
		q.ackMu.Lock()
		if q.outstanding == nil {
			q.outstanding = make(map[int64]struct{})
		}
		q.outstanding[offset] = struct{}{}
		heap.Push(&q.outstandingH, offset)
		q.ackMu.Unlock()
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

// ack processes an acknowledgement for a WAL offset. When a contiguous prefix
// of offsets becomes acknowledged, TruncateBefore is called on the WAL to
// allow file cleanup. This method must be safe to call concurrently.
func (q *Queue) ack(offset int64) {
	if q == nil || q.wal == nil {
		return
	}
	q.ackMu.Lock()
	// remove from outstanding set
	delete(q.outstanding, offset)

	// lazily pop heap until top is an outstanding member
	for q.outstandingH.Len() > 0 {
		top := q.outstandingH[0]
		if _, ok := q.outstanding[top]; ok {
			break
		}
		heap.Pop(&q.outstandingH)
	}

	var newMin int64
	if q.outstandingH.Len() == 0 {
		// no outstanding entries
		newMin = math.MaxInt64
	} else {
		newMin = q.outstandingH[0]
	}
	// only truncate if we've advanced beyond lastTruncated
	shouldTruncate := q.lastTruncated == -1 || newMin > q.lastTruncated
	if shouldTruncate {
		q.lastTruncated = newMin
	}
	q.ackMu.Unlock()

	if shouldTruncate {
		_ = q.wal.TruncateBefore(newMin)
	}
}

// doTruncate computes current smallest outstanding offset and calls WAL.TruncateBefore.
func (q *Queue) doTruncate() {
	if q == nil || q.wal == nil {
		return
	}
	q.ackMu.Lock()
	// lazily pop any heap entries already removed
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

// offsetHeap is a min-heap of int64 offsets.
type offsetHeap []int64

func (h offsetHeap) Len() int           { return len(h) }
func (h offsetHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h offsetHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *offsetHeap) Push(x any)        { *h = append(*h, x.(int64)) }
func (h *offsetHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[0 : n-1]
	return v
}

// RunBatchWorker drains up to batchSize items from the queue and invokes the handler once per batch.
func (q *Queue) RunBatchWorker(stop <-chan struct{}, batchSize int, handler func([]*Op) error) {
	if batchSize <= 0 {
		panic("queue.RunBatchWorker: batchSize must be > 0; ensure config.ValidateConfig() applied defaults")
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
