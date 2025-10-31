package types

import (
	"sync"
	"sync/atomic"
)

// HandlerID specifies the operation to perform for a queue Op.
type HandlerID string

const (
	HandlerMessageCreate HandlerID = "message.create"
	HandlerMessageUpdate HandlerID = "message.update"
	HandlerMessageDelete HandlerID = "message.delete"
	HandlerThreadCreate  HandlerID = "thread.create"
	HandlerThreadUpdate  HandlerID = "thread.update"
	HandlerThreadDelete  HandlerID = "thread.delete"
)

// RequestMetadata represents common metadata extracted from HTTP requests
type RequestMetadata struct {
	Role   string `json:"role"`
	UserID string `json:"user_id"`
	ReqID  string `json:"reqid"`
	Remote string `json:"remote"`
}

// QueueOp represents a queue operation with metadata.
type QueueOp struct {
	Handler HandlerID       // Handler to invoke
	Payload interface{}     // Payload data (may be nil, can be struct or []byte)
	TS      int64           // Timestamp (nanoseconds)
	EnqSeq  uint64          // Assigned sequence at enqueue
	Extras  RequestMetadata // Optional metadata (e.g. user id, role)
}

// WAL defines the write-ahead log interface (used by engine and WAL code).
// Simplified to match simple Log.
type WAL interface {
	Write(index uint64, data []byte) error
	Read(index uint64) (data []byte, err error)
	FirstIndex() (index uint64, err error)
	LastIndex() (index uint64, err error)
	TruncateFront(index uint64) error
	Close() error

	// WriteWithSequence writes data and returns the assigned sequence number.
	// Used when WAL is enabled to provide persistent sequence generation.
	WriteWithSequence(data []byte) (uint64, error)
}

// WALRecord holds a recovered WAL entry and its offset.
type WALRecord struct {
	Offset int64
	Data   []byte
}

// QueueItem wraps a QueueOp and buffer/queue references.
type QueueItem struct {
	Op   *QueueOp
	Sb   *SharedBuf
	once sync.Once
	Q    interface{} // Will be *queue.IngestQueue, but interface{} to avoid circular import
}

// Done manages the lifecycle of the QueueItem.
// The actual inFlight decrement logic will be handled in the queue package.
func (it *QueueItem) JobDone() {
	it.once.Do(func() {
		if it.Sb != nil {
			it.Sb.release()
			it.Sb = nil
		}
	})
}

// SetQueue sets the queue reference for a QueueItem
func (it *QueueItem) SetQueue(q interface{}) {
	it.Q = q
}

// GetQueue returns the queue reference for a QueueItem
func (it *QueueItem) GetQueue() interface{} {
	return it.Q
}

// DoOnce executes a function once for the QueueItem
func (it *QueueItem) DoOnce(fn func()) {
	it.once.Do(fn)
}

// ReleaseSharedBuf releases the shared buffer if it exists
func (it *QueueItem) ReleaseSharedBuf() {
	if it.Sb != nil {
		it.Sb.release()
		it.Sb = nil
	}
}

// SharedBuf holds data with atomic refcounting, to share between WAL and consumers.
type SharedBuf struct {
	data []byte
	refs int32
}

func (sb *SharedBuf) release() {
	atomic.AddInt32(&sb.refs, -1)
}

// BatchEntry represents an entry ready for batch application to the database.
type BatchEntry struct {
	*QueueOp
	Enq uint64
}
