package utils

import (
	"strings"

	"github.com/valyala/fasthttp"
)

// Extracts an API key from either the Authorization header or the X-API-Key header
func ExtractAPIKey(ctx *fasthttp.RequestCtx) string {
	auth := GetHeader(ctx, "Authorization")
	var key string

	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		key = strings.TrimSpace(auth[7:])
	}

	if key == "" {
		key = GetHeader(ctx, "X-API-Key")
	}

	return key
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
