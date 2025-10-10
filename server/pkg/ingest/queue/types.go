package queue

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/valyala/bytebufferpool"
)

// HandlerID identifies the concrete handler the processor should invoke for
// this Op. This is set by the enqueueing code (API layer) which has the
// authoritative intent for the operation. Processor will use Handler when
// present and will not probe payloads to determine dispatch.
type HandlerID string

const (
	HandlerMessageCreate  HandlerID = "message.create"
	HandlerMessageUpdate  HandlerID = "message.update"
	HandlerMessageDelete  HandlerID = "message.delete"
	HandlerReactionAdd    HandlerID = "reaction.add"
	HandlerReactionDelete HandlerID = "reaction.delete"
	HandlerThreadCreate   HandlerID = "thread.create"
	HandlerThreadUpdate   HandlerID = "thread.update"
	HandlerThreadDelete   HandlerID = "thread.delete"
)

// Op is a lightweight in-memory representation of a create/update/delete
// operation destined for the persistence pipeline. Payload may be backed by
// a pooled ByteBuffer; consumers must call Item.Done() when finished.
type Op struct {
	// Handler is an explicit dispatch key set by enqueueing code.
	// Processor MUST call the registered handler matching this value.
	Handler HandlerID
	Thread  string
	ID      string
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
		if it.q != nil {
			atomic.AddInt64(&it.q.inFlight, -1)
			it.q = nil
		}
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

var opPool = sync.Pool{New: func() any { return &Op{} }}
var itemPool = sync.Pool{New: func() any { return &Item{} }}

// maxPooledBuffer controls the largest buffer size that will be returned
// to the pooled ByteBuffer. Buffers larger than this will be dropped to
// avoid unbounded resident memory.
var maxPooledBuffer = 256 * 1024 // 256 KiB

// ErrQueueFull is returned by TryEnqueue when the queue is at capacity.
// Placed here so callers can reference it from the package.
var ErrQueueFull = errors.New("ingest queue full")

// ErrQueueClosed is returned when enqueue operations are attempted after the queue has closed.
var ErrQueueClosed = errors.New("ingest queue closed")
