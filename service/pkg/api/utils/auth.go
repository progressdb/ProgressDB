package utils

import (
	"strings"

	"github.com/valyala/fasthttp"
)

// ExtractAPIKey extracts API key from Authorization header or X-API-Key header
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

// GetRole returns the role from X-Role-Name header
func GetRole(ctx *fasthttp.RequestCtx) string {
	return GetHeaderLower(ctx, "X-Role-Name")
}

// GetUserID returns the user ID from X-User-ID header
func GetUserID(ctx *fasthttp.RequestCtx) string {
	return GetHeader(ctx, "X-User-ID")
}

// GetUserSignature returns the user signature from X-User-Signature header
func GetUserSignature(ctx *fasthttp.RequestCtx) string {
	return GetHeader(ctx, "X-User-Signature")
}

// IsBackendRole checks if the request has backend role
func IsBackendRole(ctx *fasthttp.RequestCtx) bool {
	return GetRole(ctx) == "backend"
}

// IsFrontendRole checks if the request has frontend role
func IsFrontendRole(ctx *fasthttp.RequestCtx) bool {
	return GetRole(ctx) == "frontend"
}

// IsAdminRole checks if the request has admin role
func IsAdminRole(ctx *fasthttp.RequestCtx) bool {
	return GetRole(ctx) == "admin"
}

// HasUserSignature checks if the request has a user signature
func HasUserSignature(ctx *fasthttp.RequestCtx) bool {
	return GetUserSignature(ctx) != ""
}
