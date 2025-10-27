package common

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"progressdb/pkg/api/router"
	"progressdb/pkg/config"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"

	"github.com/valyala/fasthttp"
)

func ExtractPayloadOrFail(ctx *fasthttp.RequestCtx) ([]byte, bool) {
	maxPayloadSize := config.GetMaxPayloadSize()
	body := ctx.PostBody()
	if len(body) == 0 {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "empty request payload")
		return nil, false
	}
	if int64(len(body)) > maxPayloadSize {
		router.WriteJSONError(ctx, fasthttp.StatusRequestEntityTooLarge, fmt.Sprintf("request payload exceeds %d bytes limit", maxPayloadSize))
		return nil, false
	}
	ref := make([]byte, len(body))
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

func NewRequestMetadata(ctx *fasthttp.RequestCtx, author string) *RequestMetadata {
	return &RequestMetadata{
		Role:   string(ctx.Request.Header.Peek("X-Role-Name")),
		UserID: author,
		ReqID:  string(ctx.Request.Header.Peek("X-Request-Id")),
		Remote: ctx.RemoteAddr().String(),
	}
}

func (rm *RequestMetadata) ToQueueExtras() QueueExtras {
	return QueueExtras{
		Role:   rm.Role,
		UserID: rm.UserID,
		ReqID:  rm.ReqID,
		Remote: rm.Remote,
	}
}

func NewEnqueueRequest(ctx *fasthttp.RequestCtx, author, threadID, messageID string, payload []byte) *EnqueueRequest {
	metadata := NewRequestMetadata(ctx, author)
	return &EnqueueRequest{
		Thread:  threadID,
		ID:      messageID,
		Payload: payload,
		TS:      0,
		Extras:  metadata.ToQueueExtras(),
	}
}

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

func ParseQueryParameters(ctx *fasthttp.RequestCtx) *QueryParameters {
	qp := &QueryParameters{
		Limit:  100,
		Cursor: strings.TrimSpace(string(ctx.QueryArgs().Peek("cursor"))),
	}

	if limStr := string(ctx.QueryArgs().Peek("limit")); limStr != "" {
		if parsedLimit, err := strconv.Atoi(limStr); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			qp.Limit = parsedLimit
		}
	}

	return qp
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
