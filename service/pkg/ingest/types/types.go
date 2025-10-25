package types

import (
	"progressdb/pkg/ingest/queue"
)

// BatchEntry represents an entry ready for batch application to the database.
type BatchEntry struct {
	*queue.QueueOp
	Enq uint64
}
