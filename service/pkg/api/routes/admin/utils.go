package admin

import (
	"encoding/json"
	"fmt"
	"progressdb/pkg/api/router"
	"progressdb/pkg/config"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"strconv"

	"github.com/valyala/fasthttp"
)

// extractPayloadOrFail standardized payload extraction and check with configurable max size
func extractPayloadOrFail(ctx *fasthttp.RequestCtx) ([]byte, bool) {
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

// validateThreadKey validates thread key format (supports both provisional and final)
func validateThreadKey(key string) error {
	if err := keys.ValidateThreadKey(key); err == nil {
		return nil
	}
	if err := keys.ValidateThreadPrvKey(key); err == nil {
		return nil
	}
	return fmt.Errorf("invalid thread key format")
}

// validateMessageKey validates message key format (supports both provisional and final)
func validateMessageKey(key string) error {
	if err := keys.ValidateMessageKey(key); err == nil {
		return nil
	}
	if err := keys.ValidateMessagePrvKey(key); err == nil {
		return nil
	}
	return fmt.Errorf("invalid message key format")
}

// extractParamOrFail Standardized path param extraction; auto-writes error and returns (string, bool)
func extractParamOrFail(ctx *fasthttp.RequestCtx, param string, missingMsg string) (string, bool) {
	val := pathParam(ctx, param)
	if val == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, missingMsg)
		return "", false
	}
	return val, true
}

// extractCursorInfoParams Standardized cursor info extraction; returns limit and cursor with defaults
func extractCursorInfoParams(ctx *fasthttp.RequestCtx) (int, string) {
	limitStr := string(ctx.QueryArgs().Peek("limit"))
	cursor := string(ctx.QueryArgs().Peek("cursor"))

	// Parse limit
	limit := 100 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	return limit, cursor
}

// pathParam extracts a path parameter from the request context
func pathParam(ctx *fasthttp.RequestCtx, param string) string {
	if v := ctx.UserValue(param); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprint(v)
	}
	return ""
}

// determineThreadIDs determines thread IDs based on request
func determineThreadIDs(ids []string, all bool) ([]string, error) {
	if all {
		vals, err := listAllThreads()
		if err != nil {
			return nil, err
		}
		threadsOut := make([]string, 0, len(vals))
		for _, raw := range vals {
			var th models.Thread
			if err := json.Unmarshal([]byte(raw), &th); err == nil {
				threadsOut = append(threadsOut, th.Key)
			}
		}
		return threadsOut, nil
	}
	return ids, nil
}

// auditSummary logs a summary of audit events
func auditSummary(event string, threads int, keys int, out map[string]map[string]string) {
	okCount := 0
	errCount := 0
	for _, m := range out {
		if s, ok := m["status"]; ok && s == "ok" {
			okCount++
		} else {
			errCount++
		}
	}
	fields := map[string]interface{}{"threads": threads, "ok": okCount, "errors": errCount}
	if keys > 0 {
		fields["keys"] = keys
	}
	auditLog(event, fields)
}

// auditLog logs audit events
func auditLog(event string, fields map[string]interface{}) {
	if logger.Audit != nil {
		attrs := make([]interface{}, 0, len(fields)*2)
		for k, v := range fields {
			attrs = append(attrs, k, v)
		}
		logger.Audit.Info(event, attrs...)
		return
	}
	attrs := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		attrs = append(attrs, k, v)
	}
	logger.Info(event, attrs...)
}

// saveThread saves thread metadata
func saveThread(threadID string, data string) error {
	key := keys.GenThreadKey(threadID)
	return storedb.SaveKey(key, []byte(data))
}
