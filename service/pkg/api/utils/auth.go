package utils

import (
	"strings"

	"github.com/valyala/fasthttp"
)

// Extracts an API key from either the Authorization header or the X-API-Key header
func ExtractAPIKey(ctx *fasthttp.RequestCtx) string {
	auth := GetHeader(ctx, "Authorization")

	// "Bearer <token>" with flexible whitespace
	if auth != "" {
		parts := strings.Fields(auth) // splits on ANY whitespace

		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1]
		}
	}

	return GetHeader(ctx, "X-API-Key")
}

// Returns the value of the X-Role-Name header, lowercased
func GetApiRole(ctx *fasthttp.RequestCtx) string {
	return GetHeaderLower(ctx, "X-Role-Name")
}

// Returns the value of the X-User-ID header
func GetUserID(ctx *fasthttp.RequestCtx) string {
	return GetHeader(ctx, "X-User-ID")
}

// Returns the value of the X-User-Signature header
func GetUserSignature(ctx *fasthttp.RequestCtx) string {
	return GetHeader(ctx, "X-User-Signature")
}

// Checks if the role in the request is "backend"
func IsBackendRole(ctx *fasthttp.RequestCtx) bool {
	return GetApiRole(ctx) == "backend"
}

// Checks if the role in the request is "frontend"
func IsFrontendRole(ctx *fasthttp.RequestCtx) bool {
	return GetApiRole(ctx) == "frontend"
}

// Checks if the role in the request is "admin"
func IsAdminRole(ctx *fasthttp.RequestCtx) bool {
	return GetApiRole(ctx) == "admin"
}

// Checks if the user signature exists in the request
func HasUserSignature(ctx *fasthttp.RequestCtx) bool {
	return GetUserSignature(ctx) != ""
}
