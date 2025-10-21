package ingest

import (
	"context"

	"progressdb/pkg/ingest/queue"
)

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
	Enq     uint64      // enqueue sequence for ordering
	Model   interface{} // unencrypted model for apply logic (*models.Message, *models.Thread, or nil)
}
