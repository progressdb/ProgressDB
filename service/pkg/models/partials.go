package models

// Partial models for operations that don't require full entity bodies.
// These are used internally in the pipeline for efficiency.

// ThreadUpdatePartial represents a partial model for thread update operations.
type ThreadUpdatePartial struct {
	ID        *string `json:"id,omitempty"`
	Title     *string `json:"title,omitempty"`
	Slug      *string `json:"slug,omitempty"`
	UpdatedTS *int64  `json:"updated_ts,omitempty"`
}

// MessageUpdatePartial represents a partial model for message update operations.
type MessageUpdatePartial struct {
	ID     *string      `json:"id,omitempty"`
	Thread *string      `json:"thread,omitempty"`
	Body   *interface{} `json:"body,omitempty"`
	TS     *int64       `json:"ts,omitempty"`
}

// ThreadDeletePartial represents a partial model for thread delete operations.
type ThreadDeletePartial struct {
	ID *string `json:"id,omitempty"`
}

// DeletePartial represents a partial model for delete operations.
// For messages, this acts as a tombstone.
type DeletePartial struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
	TS      int64  `json:"ts"`
	Thread  string `json:"thread,omitempty"` // For message deletes
	Author  string `json:"author,omitempty"` // For ownership tracking
}
