package admin

import (
	"fmt"
	"net/url"
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

func pathParam(ctx *fasthttp.RequestCtx, param string) string {
	if v := ctx.UserValue(param); v != nil {
		var s string
		if str, ok := v.(string); ok {
			s = str
		} else {
			s = fmt.Sprint(v)
		}
		// URL decode the parameter
		decoded, err := url.PathUnescape(s)
		if err != nil {
			// If decoding fails, return original value
			return s
		}
		return decoded
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
