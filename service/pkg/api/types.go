package api

import (
	"github.com/valyala/fasthttp"
)

// RequestMetadata represents common metadata extracted from HTTP requests
type RequestMetadata struct {
	Role   string `json:"role"`
	UserID string `json:"user_id"`
	ReqID  string `json:"reqid"`
	Remote string `json:"remote"`
}

// QueueExtras represents the extras map passed to queue operations
type QueueExtras map[string]string

// NewRequestMetadata extracts metadata from the request context
func NewRequestMetadata(ctx *fasthttp.RequestCtx, author string) *RequestMetadata {
	return &RequestMetadata{
		Role:   string(ctx.Request.Header.Peek("X-Role-Name")),
		UserID: author,
		ReqID:  string(ctx.Request.Header.Peek("X-Request-Id")),
		Remote: ctx.RemoteAddr().String(),
	}
}

// ToQueueExtras converts RequestMetadata to QueueExtras format
func (rm *RequestMetadata) ToQueueExtras() QueueExtras {
	return QueueExtras{
		"role":    rm.Role,
		"user_id": rm.UserID,
		"reqid":   rm.ReqID,
		"remote":  rm.Remote,
	}
}

// EnqueueRequest represents a standardized enqueue request
type EnqueueRequest struct {
	Thread  string
	ID      string
	Payload []byte
	TS      int64
	Extras  QueueExtras
}

// NewEnqueueRequest creates a standardized enqueue request
func NewEnqueueRequest(ctx *fasthttp.RequestCtx, author, threadID, messageID string, payload []byte) *EnqueueRequest {
	metadata := NewRequestMetadata(ctx, author)
	return &EnqueueRequest{
		Thread:  threadID,
		ID:      messageID,
		Payload: payload,
		TS:      0, // Will be set by caller
		Extras:  metadata.ToQueueExtras(),
	}
}
