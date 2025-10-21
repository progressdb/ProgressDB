package queue

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/bytebufferpool"
)

// opPool pools QueueOp instances to reduce allocations.
var opPool = sync.Pool{
	New: func() interface{} {
		return &QueueOp{}
	},
}

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

// QueueOp represents a queue operation with metadata.
type QueueOp struct {
	Handler   HandlerID         // Handler to invoke
	Thread    string            // Thread identifier
	ID        string            // Record identifier
	Payload   []byte            // Payload data (may be nil)
	TS        int64             // Timestamp (nanoseconds)
	EnqSeq    uint64            // Assigned sequence at enqueue
	WalOffset int64             // WAL offset, -1 if unset
	Extras    map[string]string // Optional metadata (e.g. user id, role)
}

// QueueItem wraps a QueueOp and buffer/queue references.
type QueueItem struct {
	Op   *QueueOp
	Buf  *bytebufferpool.ByteBuffer
	Sb   *SharedBuf
	once sync.Once
	Q    *IngestQueue
}

// Max buffer size for pooling. Larger ones are not pooled.
var maxPooledBuffer = 256 * 1024 // 256 KiB

const opRecordVersion = 0x1

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

	wal       WAL
	walBacked bool
}

const (
	WalModeNone = iota
	WalModeBatch
	WalModeSync
)

type IngestQueueOptions struct {
	BufferCapacity    int
	WAL               WAL
	WriteMode         string
	EnableRecovery    bool
	TruncateInterval  time.Duration
	WalBacked         bool
	DrainPollInterval time.Duration
}

// SharedBuf is a ByteBuffer with atomic refcounting, to share between WAL and consumers.
type SharedBuf struct {
	bb   *bytebufferpool.ByteBuffer
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
}

// WALRecord holds a recovered WAL entry and its offset.
type WALRecord struct {
	Offset int64
	Data   []byte
}
