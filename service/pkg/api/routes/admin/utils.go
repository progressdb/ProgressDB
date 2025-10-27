package admin

import (
	"encoding/json"
	"fmt"
	"progressdb/pkg/api/router"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"

	"github.com/valyala/fasthttp"
)

func extractParamOrFail(ctx *fasthttp.RequestCtx, param string, missingMsg string) (string, bool) {
	val := pathParam(ctx, param)
	if val == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, missingMsg)
		return "", false
	}
	return val, true
}

func pathParam(ctx *fasthttp.RequestCtx, param string) string {
	if v := ctx.UserValue(param); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprint(v)
	}
	return ""
}

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

func saveThread(threadID string, data string) error {
	key := keys.GenThreadKey(threadID)
	return storedb.SaveKey(key, []byte(data))
}
