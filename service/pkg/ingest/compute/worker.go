package compute

import (
	"context"
	"fmt"
	"sync"

	qpkg "progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/logger"
	"progressdb/pkg/state"
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
		case <-cw.stop:
			return
		default:
		}

		itm, ok := <-cw.queue.Out()
		if !ok {
			return
		}

		entries, err := cw.compute(itm.Op)
		if err != nil {
			logger.Error("compute_failed", "err", err, "op", itm.Op.MID)
			// write to failed ops file for recovery
			if writeErr := cw.failedOpWriter.WriteFailedOp(itm.Op, err); writeErr != nil {
				logger.Error("failed_op_write_failed", "err", writeErr, "op", itm.Op.MID)
			}
			itm.JobDone()
			continue
		}

		for _, entry := range entries {
			select {
			case cw.output <- entry:
			case <-cw.stop:
				itm.JobDone()
				return
			}
		}
		itm.JobDone()
	}
}

func (cw *ComputeWorker) compute(op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	switch op.Handler {
	case qpkg.HandlerMessageCreate:
		return MutMessageCreate(context.Background(), op)
	case qpkg.HandlerMessageUpdate:
		return MutMessageUpdate(context.Background(), op)
	case qpkg.HandlerMessageDelete:
		return MutMessageDelete(context.Background(), op)
	case qpkg.HandlerReactionAdd:
		return MutReactionAdd(context.Background(), op)
	case qpkg.HandlerReactionDelete:
		return MutReactionDelete(context.Background(), op)
	case qpkg.HandlerThreadCreate:
		return MutThreadCreate(context.Background(), op)
	case qpkg.HandlerThreadUpdate:
		return MutThreadUpdate(context.Background(), op)
	case qpkg.HandlerThreadDelete:
		return MutThreadDelete(context.Background(), op)
	default:
		return nil, fmt.Errorf("unknown handler: %s", op.Handler)
	}
}
