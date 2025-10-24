package types

import (
	"progressdb/pkg/ingest/queue"
)

// BatchEntry represents an entry ready for batch application to the database.
type BatchEntry struct {
	Handler queue.HandlerID
	TID     string
	MID     string
	Payload []byte
	TS      int64
	Enq     uint64
	Model   interface{} // *models.Message or *models.Thread
	Author  string      // Author ID for index updates
}
