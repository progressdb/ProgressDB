package common

import (
	"fmt"
	"strconv"

	"progressdb/pkg/api/router"
	"progressdb/pkg/config"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/state/telemetry"
	"progressdb/pkg/store/keys"

	"github.com/valyala/fasthttp"
)

func ExtractPayloadOrFail(ctx *fasthttp.RequestCtx) ([]byte, bool) {
	maxPayloadSize := config.GetMaxPayloadSize()
	body := ctx.PostBody()
	bodyLen := int64(len(body))

	// Log the body length
	logger.Info("ExtractPayloadOrFail.body_length", bodyLen)

	if bodyLen == 0 {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "empty request payload")
		return nil, false
	}
	if bodyLen > maxPayloadSize {
		router.WriteJSONError(ctx, fasthttp.StatusRequestEntityTooLarge, fmt.Sprintf("request payload exceeds %d bytes limit", maxPayloadSize))
		return nil, false
	}
	ref := make([]byte, bodyLen)
	copy(ref, body)
	return ref, true
}

func ValidateThreadKey(key string) error {
	if err := keys.ValidateThreadKey(key); err == nil {
		return nil
	}
	if err := keys.ValidateThreadPrvKey(key); err == nil {
		return nil
	}
	return fmt.Errorf("invalid thread key format")
}

func ValidateMessageKey(key string) error {
	if err := keys.ValidateMessageKey(key); err == nil {
		return nil
	}
	if err := keys.ValidateMessagePrvKey(key); err == nil {
		return nil
	}
	return fmt.Errorf("invalid message key format")
}

func ExtractParamOrFail(ctx *fasthttp.RequestCtx, param string, missingMsg string) (string, bool) {
	val := PathParam(ctx, param)
	if val == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, missingMsg)
		return "", false
	}
	return val, true
}

func ExtractCursorInfoParams(ctx *fasthttp.RequestCtx) (int, string) {
	limitStr := string(ctx.QueryArgs().Peek("limit"))
	cursor := string(ctx.QueryArgs().Peek("cursor"))

	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	return limit, cursor
}

func PathParam(ctx *fasthttp.RequestCtx, param string) string {
	if v := ctx.UserValue(param); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprint(v)
	}
	return ""
}

// NewRequestMetadata extracts metadata from the request context
func NewRequestMetadata(ctx *fasthttp.RequestCtx, author string) *RequestMetadata {
	return &RequestMetadata{
		Role:   string(ctx.Request.Header.Peek("X-Role-Name")),
		UserID: author,
		ReqID:  string(ctx.Request.Header.Peek("X-Request-Id")),
		Remote: ctx.RemoteAddr().String(),
	}
}

// ToQueueExtras converts RequestMetadata to strongly-typed QueueExtras
func (rm *RequestMetadata) ToQueueExtras() QueueExtras {
	return QueueExtras{
		Role:   rm.Role,
		UserID: rm.UserID,
		ReqID:  rm.ReqID,
		Remote: rm.Remote,
	}
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
func ValidatePathParam(ctx *fasthttp.RequestCtx, paramName string) (string, bool) {
	value := PathParam(ctx, paramName)
	if value == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, paramName+" missing")
		return "", false
	}
	return value, true
}

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
