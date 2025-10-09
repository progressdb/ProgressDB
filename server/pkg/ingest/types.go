package ingest

import (
	"context"

	"progressdb/pkg/ingest/queue"
)

// ProcessorFunc is the signature for operation handlers used by the
// ingest processor. Handlers receive the unmarshalled Op and are expected
// to perform persistence work (or prepare DB entries) synchronously. They
// must return an error when the operation should be retried.
// ProcessorFunc processes a single Op and returns zero or more BatchEntry
// objects to be applied together in a batch. Returning an error signals a
// transient handler failure; the processor will log and continue.
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
