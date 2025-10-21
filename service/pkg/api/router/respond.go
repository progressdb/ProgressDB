package router

import (
	"encoding/json"

	"github.com/valyala/fasthttp"
)

// WriteJSON writes a JSON response.
func WriteJSON(ctx *fasthttp.RequestCtx, data interface{}) error {
	ctx.Response.Header.Set("Content-Type", "application/json")
	return json.NewEncoder(ctx).Encode(data)
}

// WriteJSONError writes a JSON error response.
func WriteJSONError(ctx *fasthttp.RequestCtx, status int, message string) {
	ctx.SetStatusCode(status)
	ctx.Response.Header.Set("Content-Type", "application/json")
	_ = json.NewEncoder(ctx).Encode(map[string]string{"error": message})
}

// WriteJSONOk writes a simple OK JSON response.
func WriteJSONOk(ctx *fasthttp.RequestCtx, data map[string]interface{}) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	_ = json.NewEncoder(ctx).Encode(data)
}

// ToRawMessages converts a slice of JSON-encoded strings to a slice of json.RawMessage.
func ToRawMessages(vals []string) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(vals))
	for _, s := range vals {
		out = append(out, json.RawMessage(s))
	}
	return out
}
