package compute

import (
	"context"
	"fmt"
	"sync"

	qpkg "progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/state"
	"progressdb/pkg/state/logger"
)

// ComputeWorker handles transformation of QueueOp to types.BatchEntry.
type ComputeWorker struct {
	queue          *qpkg.IngestQueue
	output         chan<- types.BatchEntry
	stop           <-chan struct{}
	workers        int
	failedOpWriter *state.FailedOpWriter
}

// NewComputeWorker creates a new compute worker.
func NewComputeWorker(queue *qpkg.IngestQueue, output chan<- types.BatchEntry, workers int, failedOpsPath string) *ComputeWorker {
	return &ComputeWorker{
		queue:          queue,
		output:         output,
		workers:        workers,
		failedOpWriter: state.NewFailedOpWriter(failedOpsPath),
	}
}

// Start begins the compute workers.
func (cw *ComputeWorker) Start(stop <-chan struct{}, wg *sync.WaitGroup) {
	cw.stop = stop
	for i := 0; i < cw.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cw.run()
		}()
	}
}

func (cw *ComputeWorker) run() {
	for {
		select {
		case <-cw.stop: // check for stop signal
			return
		default:
		}

		itm, ok := <-cw.queue.Out() // get item from queue
		if !ok {
			return
		}

		entries, err := cw.compute(itm.Op) // run compute function
		if err != nil {
			// write failed op for recovery
			if writeErr := cw.failedOpWriter.WriteFailedOp(itm.Op, err); writeErr != nil {
				logger.Error("failed_op_write_failed", "err", writeErr, "handler", itm.Op.Handler)
			}
			itm.JobDone() // mark job done on error
			continue
		}

		for _, entry := range entries {
			select {
			case cw.output <- entry: // send result to output
			case <-cw.stop: // exit on stop
				itm.JobDone()
				return
			}
		}
		itm.JobDone() // mark job done after processing
	}
}

func (cw *ComputeWorker) compute(op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	switch op.Handler {
	case qpkg.HandlerMessageCreate:
		return ComputeMessageCreate(context.Background(), op)
	case qpkg.HandlerMessageUpdate:
		return ComputeMessageUpdate(context.Background(), op)
	case qpkg.HandlerMessageDelete:
		return ComputeMessageDelete(context.Background(), op)
	case qpkg.HandlerThreadCreate:
		return ComputeThreadCreate(context.Background(), op)
	case qpkg.HandlerThreadUpdate:
		return ComputeThreadUpdate(context.Background(), op)
	case qpkg.HandlerThreadDelete:
		return ComputeThreadDelete(context.Background(), op)
	default:
		return nil, fmt.Errorf("unknown handler: %s", op.Handler)
	}
}
