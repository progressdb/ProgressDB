package models

type PaginationRequest struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

type PaginationResponse struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
	Count      int    `json:"count"`
	Total      int    `json:"total,omitempty"`
}

type MessageCursor struct {
	ThreadID  string `json:"thread_id"`
	Timestamp int64  `json:"timestamp"`
	Sequence  uint64 `json:"sequence"`
}

type ThreadCursor struct {
	UserID    string `json:"user_id"`
	Timestamp int64  `json:"timestamp"`
	ThreadID  string `json:"thread_id"`
}

type ReadRequestCursorInfo struct {
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

type ReadResponseCursorInfo struct {
	Cursor     string `json:"cursor"`
	HasMore    bool   `json:"has_more"`
	TotalCount uint64 `json:"total_count"`
	LastSeq    uint64 `json:"last_seq"`
}
