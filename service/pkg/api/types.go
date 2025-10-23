package api

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"progressdb/pkg/models"
	"progressdb/pkg/telemetry"

	"github.com/valyala/fasthttp"
	"progressdb/pkg/api/router"
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

// Response types for standardized API responses
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

type ReactionsResponse struct {
	ID        string      `json:"id"`
	Reactions interface{} `json:"reactions"`
}

// PaginationMeta contains pagination metadata
type PaginationMeta struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
	Count      int    `json:"count"`
}

// MessageCursor represents cursor data for message pagination
type MessageCursor struct {
	ThreadID  string `json:"thread_id"`
	Timestamp int64  `json:"timestamp"`
	Sequence  uint64 `json:"sequence"`
}

// ThreadCursor represents cursor data for thread pagination
type ThreadCursor struct {
	UserID    string `json:"user_id"`
	Timestamp int64  `json:"timestamp"`
	ThreadID  string `json:"thread_id"`
}

// EncodeMessageCursor encodes message cursor data to opaque token
func EncodeMessageCursor(threadID string, timestamp int64, sequence uint64) (string, error) {
	cursor := MessageCursor{
		ThreadID:  threadID,
		Timestamp: timestamp,
		Sequence:  sequence,
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// DecodeMessageCursor decodes message cursor token to data
func DecodeMessageCursor(cursor string) (*MessageCursor, error) {
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	var mc MessageCursor
	err = json.Unmarshal(data, &mc)
	if err != nil {
		return nil, err
	}
	return &mc, nil
}

// EncodeThreadCursor encodes thread cursor data to opaque token
func EncodeThreadCursor(userID, threadID string, timestamp int64) (string, error) {
	cursor := ThreadCursor{
		UserID:    userID,
		Timestamp: timestamp,
		ThreadID:  threadID,
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// DecodeThreadCursor decodes thread cursor token to data
func DecodeThreadCursor(cursor string) (*ThreadCursor, error) {
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	var tc ThreadCursor
	err = json.Unmarshal(data, &tc)
	if err != nil {
		return nil, err
	}
	return &tc, nil
}

// QueryParameters represents common query parameters
type QueryParameters struct {
	Limit  int
	Cursor string
}

// ReadRequestCursorInfo represents cursor information for read requests
type ReadRequestCursorInfo struct {
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

// ReadResponseCursorInfo represents cursor information for read responses
type ReadResponseCursorInfo struct {
	Cursor  string `json:"cursor"`
	HasMore bool   `json:"has_more"`
}

// ParseQueryParameters extracts common query parameters from request
func ParseQueryParameters(ctx *fasthttp.RequestCtx) *QueryParameters {
	qp := &QueryParameters{
		Limit:  100, // Default limit
		Cursor: strings.TrimSpace(string(ctx.QueryArgs().Peek("cursor"))),
	}

	if limStr := string(ctx.QueryArgs().Peek("limit")); limStr != "" {
		if parsedLimit, err := strconv.Atoi(limStr); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			qp.Limit = parsedLimit
		}
	}

	return qp
}

// ValidatePathParam validates and extracts path parameter
func ValidatePathParam(ctx *fasthttp.RequestCtx, paramName string) (string, bool) {
	value := pathParam(ctx, paramName)
	if value == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, paramName+" missing")
		return "", false
	}
	return value, true
}

// SetupReadHandler standardizes read handler setup
func SetupReadHandler(ctx *fasthttp.RequestCtx, operationName string) (string, *telemetry.Trace, bool) {
	tr := telemetry.Track("api." + operationName)
	ctx.Response.Header.Set("Content-Type", "application/json")

	author, authErr := ValidateAuthor(ctx, "")
	if authErr != nil {
		WriteValidationError(ctx, authErr)
		tr.Finish()
		return "", nil, false
	}

	return author, tr, true
}
