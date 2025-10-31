package admin

import (
	"fmt"
	"progressdb/pkg/api/router"
	storedb "progressdb/pkg/store/db/storedb"
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

// pathParam returns the parameter as-is (no additional URL decoding).
func pathParam(ctx *fasthttp.RequestCtx, param string) string {
	if v := ctx.UserValue(param); v != nil {
		if str, ok := v.(string); ok {
			return str
		}
		return fmt.Sprint(v)
	}
	return ""
}

func saveThread(threadKey string, data string) error {
	key := keys.GenThreadKey(threadKey)
	return storedb.SaveKey(key, []byte(data))
}

func extractQueryOrFail(ctx *fasthttp.RequestCtx, param string, missingMsg string) (string, bool) {
	val := string(ctx.QueryArgs().Peek(param))
	if val == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, missingMsg)
		return "", false
	}
	return val, true
}
