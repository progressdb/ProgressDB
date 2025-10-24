package queue

import (
	"sync"
	"sync/atomic"
	"time"
)

// HandlerID specifies the operation to perform for a queue Op.
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

// RequestMetadata represents common metadata extracted from HTTP requests
type RequestMetadata struct {
	Role   string `json:"role"`
	UserID string `json:"user_id"`
	ReqID  string `json:"reqid"`
	Remote string `json:"remote"`
}

// QueueOp represents a queue operation with metadata.
type QueueOp struct {
	Handler   HandlerID       // Handler to invoke
	TID       string          // Thread ID
	MID       string          // Message ID
	Payload   []byte          // Payload data (may be nil)
	TS        int64           // Timestamp (nanoseconds)
	EnqSeq    uint64          // Assigned sequence at enqueue (used when WAL disabled)
	WalSeq    uint64          // WAL-assigned sequence (used when WAL enabled)
	WalOffset int64           // WAL offset, -1 if unset
	Extras    RequestMetadata // Optional metadata (e.g. user id, role)
}

// QueueItem wraps a QueueOp and buffer/queue references.
type QueueItem struct {
	Op   *QueueOp
	Sb   *SharedBuf
	once sync.Once
	Q    *IngestQueue
}

// Done manages the lifecycle of the QueueItem, decrementing inFlight and releasing resources.
func (it *QueueItem) JobDone() {
	it.once.Do(func() {
		if it.Q != nil {
			atomic.AddInt64(&it.Q.inFlight, -1)
			it.Q = nil
		}
		if it.Sb != nil {
			it.Sb.release()
			it.Sb = nil
		}
	})
}

// Queue is the core queue/engine type used package-wide.
type IngestQueue struct {
	ch                chan *QueueItem
	capacity          int
	dropped           uint64
	closed            int32
	drainPollInterval time.Duration

	enqWg     sync.WaitGroup
	closeOnce sync.Once
	inFlight  int64
	enqMu     sync.Mutex // protects enqueue operations

	wal       WAL
	walBacked bool
}

// SharedBuf holds data with atomic refcounting, to share between WAL and consumers.
type SharedBuf struct {
	data []byte
	refs int32
}

func (sb *SharedBuf) release() {
	atomic.AddInt32(&sb.refs, -1)
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
