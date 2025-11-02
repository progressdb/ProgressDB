package logger

import (
	"strings"
	"unicode/utf8"

	"github.com/valyala/fasthttp"
)

func maskedValue(v string) string {
	if v == "" {
		return ""
	}
	l := utf8.RuneCountInString(v)
	if l <= 2 {
		return "<redacted>"
	}
	first, _ := utf8.DecodeRuneInString(v)
	last, _ := utf8.DecodeLastRuneInString(v)
	return string(first) + "*****" + string(last)
}

func redactHeaderValue(_ string, v string) string {
	if v == "" {
		return ""
	}
	return maskedValue(v)
}
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
	fullURL := string(ctx.URI().FullURI())
	Info("incoming_request",
		"method", string(ctx.Method()),
		"remote", ctx.RemoteAddr().String(),
		"url", fullURL,
	)
}
