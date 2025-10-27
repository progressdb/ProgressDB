package common

import (
	"progressdb/pkg/models"
)

type RequestMetadata struct {
	Role   string `json:"role"`
	UserID string `json:"user_id"`
	ReqID  string `json:"reqid"`
	Remote string `json:"remote"`
}

type QueueExtras struct {
	Role   string `json:"role,omitempty"`
	UserID string `json:"user_id"`
	ReqID  string `json:"reqid"`
	Remote string `json:"remote,omitempty"`
}

type EnqueueRequest struct {
	Thread  string
	ID      string
	Payload []byte
	TS      int64
	Extras  QueueExtras
}

type ThreadsListResponse struct {
	Threads    []models.Thread `json:"threads"`
	Pagination PaginationMeta  `json:"pagination"`
}

type ThreadResponse struct {
	Thread models.Thread `json:"thread"`
}

type MessagesListResponse struct {
	Thread     string           `json:"thread"`
	Messages   []models.Message `json:"messages"`
	Metadata   interface{}      `json:"metadata,omitempty"`
	Pagination PaginationMeta   `json:"pagination"`
}

type MessageResponse struct {
	Message models.Message `json:"message"`
}

type PaginationMeta struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
	Count      int    `json:"count"`
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

type QueryParameters struct {
	Limit  int
	Cursor string
}

type ReadRequestCursorInfo struct {
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

type ReadResponseCursorInfo struct {
	Cursor  string `json:"cursor"`
	HasMore bool   `json:"has_more"`
}
