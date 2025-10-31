package utils

import (
	"strconv"
	"strings"

	"github.com/valyala/fasthttp"
)

// Header utilities

// GetHeader returns header value with trimming
func GetHeader(ctx *fasthttp.RequestCtx, key string) string {
	return strings.TrimSpace(string(ctx.Request.Header.Peek(key)))
}

// GetHeaderLower returns header value with trimming and lowercase
func GetHeaderLower(ctx *fasthttp.RequestCtx, key string) string {
	return strings.ToLower(GetHeader(ctx, key))
}

// GetHeaderInt returns header value as integer, with default fallback
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

// Query parameter utilities

// GetQuery returns query parameter value with trimming
func GetQuery(ctx *fasthttp.RequestCtx, key string) string {
	return strings.TrimSpace(string(ctx.QueryArgs().Peek(key)))
}

// GetQueryLower returns query parameter value with trimming and lowercase
func GetQueryLower(ctx *fasthttp.RequestCtx, key string) string {
	return strings.ToLower(GetQuery(ctx, key))
}

// GetQueryInt returns query parameter value as integer, with default fallback
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

// Path parameter utilities

// GetPathParam returns path parameter value
func GetPathParam(ctx *fasthttp.RequestCtx, param string) string {
	if v := ctx.UserValue(param); v != nil {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(v.(string), "/"), "/"))
	}
	return ""
}

// GetPathParamLower returns path parameter value with lowercase
func GetPathParamLower(ctx *fasthttp.RequestCtx, param string) string {
	return strings.ToLower(GetPathParam(ctx, param))
}

// GetPathParamInt returns path parameter value as integer, with default fallback
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
