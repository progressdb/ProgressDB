package admin

import (
	"progressdb/pkg/api/router"
	"progressdb/pkg/api/utils"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"

	"github.com/valyala/fasthttp"
)

func extractParamOrFail(ctx *fasthttp.RequestCtx, param string, missingMsg string) (string, bool) {
	val := utils.GetPathParam(ctx, param)
	if val == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, missingMsg)
		return "", false
	}
	return val, true
}

func saveThread(threadKey string, data string) error {
	key := keys.GenThreadKey(threadKey)
	return storedb.SaveKey(key, []byte(data))
}

func extractQueryOrFail(ctx *fasthttp.RequestCtx, param string, missingMsg string) (string, bool) {
	val := utils.GetQuery(ctx, param)
	if val == "" {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, missingMsg)
		return "", false
	}
	return val, true
}
