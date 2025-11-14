package utils

import (
	"strconv"
	"strings"

	"github.com/valyala/fasthttp"
)

func GetHeader(ctx *fasthttp.RequestCtx, key string) string {
	return strings.TrimSpace(string(ctx.Request.Header.Peek(key)))
}

func GetHeaderLower(ctx *fasthttp.RequestCtx, key string) string {
	return strings.ToLower(GetHeader(ctx, key))
}

func GetHeaderInt(ctx *fasthttp.RequestCtx, key string, defaultValue int) int {
	value := GetHeader(ctx, key)
	if value == "" {
		return defaultValue
	}
	if intValue, err := strconv.Atoi(value); err == nil {
		return intValue
	}
	return defaultValue
}

func GetQuery(ctx *fasthttp.RequestCtx, key string) string {
	return strings.TrimSpace(string(ctx.QueryArgs().Peek(key)))
}

func GetQueryLower(ctx *fasthttp.RequestCtx, key string) string {
	return strings.ToLower(GetQuery(ctx, key))
}

func GetQueryInt(ctx *fasthttp.RequestCtx, key string, defaultValue int) int {
	value := GetQuery(ctx, key)
	if value == "" {
		return defaultValue
	}
	if intValue, err := strconv.Atoi(value); err == nil {
		return intValue
	}
	return defaultValue
}

func GetPathParam(ctx *fasthttp.RequestCtx, param string) string {
	if v := ctx.UserValue(param); v != nil {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(v.(string), "/"), "/"))
	}
	return ""
}

func GetPathParamLower(ctx *fasthttp.RequestCtx, param string) string {
	return strings.ToLower(GetPathParam(ctx, param))
}

func GetPathParamInt(ctx *fasthttp.RequestCtx, param string, defaultValue int) int {
	value := GetPathParam(ctx, param)
	if value == "" {
		return defaultValue
	}
	if intValue, err := strconv.Atoi(value); err == nil {
		return intValue
	}
	return defaultValue
}
