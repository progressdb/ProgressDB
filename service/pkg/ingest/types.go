package ingest

import (
	"context"

	"progressdb/pkg/ingest/queue"
)

// handles an Op, returning batch entries or an error for retry.
type ProcessorFunc func(ctx context.Context, op *queue.QueueOp) ([]BatchEntry, error)

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
