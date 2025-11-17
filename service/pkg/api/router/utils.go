package router

import (
	"fmt"

	"progressdb/pkg/api/utils"
	"progressdb/pkg/state/telemetry"
	"progressdb/pkg/store/keys"

	"github.com/valyala/fasthttp"
)

func ExtractPayloadOrFail(ctx *fasthttp.RequestCtx) ([]byte, bool) {
	body := ctx.PostBody()
	bodyLen := int64(len(body))
	ref := make([]byte, bodyLen)
	copy(ref, body)
	return ref, true
}

func ValidateThreadKey(key string) error {
	parsed, err := keys.ParseKey(key)
	if err != nil {
		return fmt.Errorf("invalid thread key format: %w", err)
	}
	if parsed.Type != keys.KeyTypeThread {
		return fmt.Errorf("expected thread key, got %s", parsed.Type)
	}
	return nil
}

func ValidateMessageKey(key string) error {
	parsed, err := keys.ParseKey(key)
	if err != nil {
		return fmt.Errorf("invalid message key format: %w", err)
	}
	if parsed.Type != keys.KeyTypeMessage && parsed.Type != keys.KeyTypeMessageProvisional {
		return fmt.Errorf("expected message key, got %s", parsed.Type)
	}
	return nil
}

func ExtractParamOrFail(ctx *fasthttp.RequestCtx, param string, missingMsg string) (string, bool) {
	val := PathParam(ctx, param)
	if val == "" {
		WriteJSONError(ctx, fasthttp.StatusBadRequest, missingMsg)
		return "", false
	}
	return val, true
}

func ExtractCursorInfoParams(ctx *fasthttp.RequestCtx) (int, string) {
	limit := utils.GetQueryInt(ctx, "limit", 100)
	cursor := utils.GetQuery(ctx, "cursor")
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
		ApiRole: utils.GetApiRole(ctx),
		UserID:  author,
		ReqID:   utils.GetHeader(ctx, "X-Request-Id"),
		ReqIP:   ctx.RemoteAddr().String(),
	}
}

// ToQueueExtras converts RequestMetadata to strongly-typed QueueExtras
func (rm *RequestMetadata) ToQueueExtras() QueueExtras {
	return QueueExtras{
		ApiRole: rm.ApiRole,
		UserID:  rm.UserID,
		ReqID:   rm.ReqID,
		ReqIP:   rm.ReqIP,
	}
}

// NewEnqueueRequest creates a standardized enqueue request
func NewEnqueueRequest(ctx *fasthttp.RequestCtx, author, threadKey, messageKey string, payload []byte) *EnqueueRequest {
	metadata := NewRequestMetadata(ctx, author)
	return &EnqueueRequest{
		Thread:  threadKey,
		ID:      messageKey,
		Payload: payload,
		TS:      0, // Will be set by caller
		Extras:  metadata.ToQueueExtras(),
	}
}
func ValidatePathParam(ctx *fasthttp.RequestCtx, paramName string) (string, bool) {
	value := PathParam(ctx, paramName)
	if value == "" {
		WriteJSONError(ctx, fasthttp.StatusBadRequest, paramName+" missing")
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
