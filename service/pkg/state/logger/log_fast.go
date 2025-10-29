package logger

import (
	"strings"

	"github.com/valyala/fasthttp"
)

func SafeHeadersFast(ctx *fasthttp.RequestCtx) string {
	parts := make([]string, 0)
	ctx.Request.Header.VisitAll(func(k, v []byte) {
		key := string(k)
		val := redactHeaderValue(key, string(v))
		parts = append(parts, key+"="+val)
	})
	return strings.Join(parts, "; ")
}

func LogRequestFast(ctx *fasthttp.RequestCtx) {
	if Log == nil {
		return
	}
	Info("incoming_request", "method", string(ctx.Method()), "path", string(ctx.Path()), "remote", ctx.RemoteAddr().String())
}
