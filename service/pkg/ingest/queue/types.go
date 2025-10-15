package queue

import (
	"context"
	"hash/crc32"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/bytebufferpool"
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
	walMode   int
	walBacked bool

	ackMu         sync.Mutex
	outstanding   map[int64]struct{}
	outstandingH  offsetHeap
	lastTruncated int64
}

const (
	WalModeNone = iota
	WalModeBatch
	WalModeSync
)

type IngestQueueOptions struct {
	Capacity          int
	WAL               WAL
	Mode              string
	Recover           bool
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
type WAL interface {
	Append([]byte) (int64, error)
	AppendCtx([]byte, context.Context) (int64, error)
	AppendPooled(*SharedBuf) (int64, error)
	AppendPooledCtx(*SharedBuf, context.Context) (int64, error)
	AppendSync([]byte) (int64, error)
	Flush() error
	Recover() ([]WALRecord, error)
	RecoverStream(func(WALRecord) error) error
	TruncateBefore(int64) error
	Close() error
}

// WALRecord holds a recovered WAL entry and its offset.
type WALRecord struct {
	Offset int64
	Data   []byte
}

// Options configures WAL behavior when creating WAL instances.
// DurableWALConfigOptions configures WAL behavior when creating WAL instances.
type DurableWALConfigOptions struct {
	Dir              string
	MaxFileSize      int64
	EnableBatch      bool
	BatchSize        int
	BatchInterval    time.Duration
	EnableCompress   bool
	CompressMinBytes int64
}

// walFile holds file info for WAL segments.
type DurableFileSegment struct {
	f            *os.File
	num          int
	offset       int64
	size         int64
	minSeq       int64 // Minimum sequence in file
	maxSeq       int64 // Maximum sequence in file
	fileChecksum uint32
}

// batchBuffer accumulates pending batch writes.
type DurableBatchBuffer struct {
	entries []DurableBatchEntry
	size    int64
}

type DurableBatchEntry struct {
	seq  int64
	data []byte
	sb   *SharedBuf // Pooled buffer ownership (optional)
}

// FileWAL implements the WAL interface.
type DurableFile struct {
	dir              string
	maxSize          int64
	enableBatch      bool
	batchSize        int
	batchInterval    time.Duration
	enableCompress   bool
	compressMinBytes int64
	compressMinRatio float64

	// Buffering/backpressure controls
	maxBufferedBytes   int64
	maxBufferedEntries int
	bufferWaitTimeout  time.Duration
	spaceCh            chan struct{}

	mu       sync.Mutex
	curr     *DurableFileSegment
	files    []*DurableFileSegment
	nextNum  int
	seq      int64
	crcTable *crc32.Table

	// Batch mode state
	batch      *DurableBatchBuffer
	batchTimer *time.Timer
	flushCond  *sync.Cond
	closed     bool
}
