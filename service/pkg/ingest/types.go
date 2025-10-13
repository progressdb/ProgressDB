package ingest

import (
	"context"

	"progressdb/pkg/ingest/queue"
)

// ProcessorFunc handles an Op, returning batch entries or an error for retry.
type ProcessorFunc func(ctx context.Context, op *queue.Op) ([]BatchEntry, error)

// BatchEntry represents a single operation prepared for batch apply.
type BatchEntry struct {
	// Handler identifies the originating handler for this batch entry.
	Handler queue.HandlerID
	Thread  string
	MsgID   string
	Payload []byte
	TS      int64
	Enq     uint64 // enqueue sequence for ordering
}

// context key for thread metadata prefetch map
type threadMetaKeyType struct{}

var threadMetaKey threadMetaKeyType
