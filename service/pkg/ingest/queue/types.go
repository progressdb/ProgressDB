package queue

import (
	"sync"
	"sync/atomic"
	"time"

	"progressdb/pkg/ingest/types"
)

// Queue is the core queue/engine type used package-wide.
type IngestQueue struct {
	ch                chan *types.QueueItem
	capacity          int
	dropped           uint64
	closed            int32
	drainPollInterval time.Duration

	enqWg     sync.WaitGroup
	closeOnce sync.Once
	inFlight  int64
	enqMu     sync.Mutex // protects enqueue operations

	wal       types.WAL
	walBacked bool
}

// JobDone manages the lifecycle of a QueueItem for this queue.
func (q *IngestQueue) JobDone(item *types.QueueItem) {
	item.DoOnce(func() {
		atomic.AddInt64(&q.inFlight, -1)
		item.SetQueue(nil)
		item.ReleaseSharedBuf()
	})
}
