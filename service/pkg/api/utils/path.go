package utils

import (
	"strings"

	"github.com/valyala/fasthttp"
)

// returns the request path as string
func GetPath(ctx *fasthttp.RequestCtx) string {
	return string(ctx.Path())
}

// checks if the request path starts with the given prefix
func HasPathPrefix(ctx *fasthttp.RequestCtx, prefix string) bool {
	return strings.HasPrefix(GetPath(ctx), prefix)
}

// checks if the request path exactly matches the given path
func HasPath(ctx *fasthttp.RequestCtx, path string) bool {
	return GetPath(ctx) == path
}

// returns the path split into segments
func GetPathSegments(ctx *fasthttp.RequestCtx) []string {
	path := strings.Trim(GetPath(ctx), "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}
