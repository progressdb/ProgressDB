package utils

import (
	"encoding/json"

	"github.com/valyala/fasthttp"
)

// JSONErrorFast writes a JSON error response using fasthttp.
func JSONErrorFast(ctx *fasthttp.RequestCtx, status int, message string) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(status)
	_ = json.NewEncoder(ctx).Encode(map[string]string{"error": message})
}

// JSONWriteFast writes a JSON response with the provided status code.
func JSONWriteFast(ctx *fasthttp.RequestCtx, status int, v interface{}) error {
	ctx.SetContentType("application/json")
	if status != 0 {
		ctx.SetStatusCode(status)
	}
	return json.NewEncoder(ctx).Encode(v)
}
