package ingest

import (
	"context"
	"sync"
	"time"

	"progressdb/pkg/ingest/queue"
)

type Ingestor struct {
	q        *queue.IngestQueue
	workers  int
	stop     chan struct{}
	wg       sync.WaitGroup
	running  int32
	handlers map[queue.HandlerID]IngestorFunc

	// batch knobs (future)
	maxBatch int
	flushDur time.Duration
	// pause state
	paused int32

	seqCounter uint64
	nextCommit uint64
	commitMu   sync.Mutex
	commitCond *sync.Cond
}

// handles an Op, returning batch entries or an error for retry.
type IngestorFunc func(ctx context.Context, op *queue.QueueOp) ([]BatchEntry, error)

// represents a single operation prepared for batch apply.
type BatchEntry struct {
	// Handler identifies the originating handler for this batch entry.
	Handler queue.HandlerID
	Thread  string
	MsgID   string
	Payload []byte
	TS      int64
	Enq     uint64 // enqueue sequence for ordering
}
