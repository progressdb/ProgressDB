package api

import (
	"strconv"

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
	Threads []models.Thread `json:"threads"`
}

type ThreadResponse struct {
	Thread models.Thread `json:"thread"`
}

type MessagesListResponse struct {
	Thread   string           `json:"thread"`
	Messages []models.Message `json:"messages"`
	Metadata interface{}      `json:"metadata,omitempty"`
}

type MessageResponse struct {
	Message models.Message `json:"message"`
}

type ReactionsResponse struct {
	ID        string      `json:"id"`
	Reactions interface{} `json:"reactions"`
}

// QueryParameters represents common query parameters
type QueryParameters struct {
	Limit int
}

// ParseQueryParameters extracts common query parameters from request
func ParseQueryParameters(ctx *fasthttp.RequestCtx) *QueryParameters {
	qp := &QueryParameters{
		Limit: -1, 
	}

	if limStr := string(ctx.QueryArgs().Peek("limit")); limStr != "" {
		if parsedLimit, err := strconv.Atoi(limStr); err == nil && parsedLimit >= 0 {
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
