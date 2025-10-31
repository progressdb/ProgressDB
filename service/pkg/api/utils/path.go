package utils

import (
	"strings"

	"github.com/valyala/fasthttp"
)

// GetPath returns the request path as string
func GetPath(ctx *fasthttp.RequestCtx) string {
	return string(ctx.Path())
}

// HasPathPrefix checks if the request path starts with the given prefix
func HasPathPrefix(ctx *fasthttp.RequestCtx, prefix string) bool {
	return strings.HasPrefix(GetPath(ctx), prefix)
}

// HasPath checks if the request path exactly matches the given path
func HasPath(ctx *fasthttp.RequestCtx, path string) bool {
	return GetPath(ctx) == path
}

// GetPathSegments returns the path split into segments
func GetPathSegments(ctx *fasthttp.RequestCtx) []string {
	path := strings.Trim(GetPath(ctx), "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}
