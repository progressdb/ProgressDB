package backend

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/auth"
	"progressdb/pkg/api/router"
	"progressdb/pkg/state/logger"
)

func Sign(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	logger.Info("signHandler called", "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))

	role := string(ctx.Request.Header.Peek("X-Role-Name"))
	if role != "backend" {
		logger.Warn("forbidden: non-backend role attempted to sign", "role", role, "remote", ctx.RemoteAddr().String())
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}

	key := getAuthorizationAPIKey(ctx)
	if key == "" {
		logger.Warn("missing api key in signHandler", "remote", ctx.RemoteAddr().String())
		router.WriteJSONError(ctx, fasthttp.StatusUnauthorized, "missing api key")
		return
	}

	var payload struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&payload); err != nil {
		logger.Warn("invalid JSON payload in signHandler", "error", err, "remote", ctx.RemoteAddr().String())
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid JSON payload")
		return
	}

	// Validate user ID format and content
	if err := ValidateUserID(payload.UserID); err != nil {
		logger.Warn("invalid user ID in signHandler", "error", err, "user_id", payload.UserID, "remote", ctx.RemoteAddr().String())
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid user ID: %s", err.Error()))
		return
	}

	logger.Info("signing userId", "remote", ctx.RemoteAddr().String(), "role", role)
	sig := auth.CreateHMACSignature(payload.UserID, key)

	if err := router.WriteJSON(ctx, map[string]string{"userId": payload.UserID, "signature": sig}); err != nil {
		logger.Error("failed to encode signHandler response", "error", err, "remote", ctx.RemoteAddr().String())
	}
}

func getAuthorizationAPIKey(ctx *fasthttp.RequestCtx) string {
	auth := string(ctx.Request.Header.Peek("Authorization"))
	var key string
	if len(auth) > 7 && (auth[:7] == "Bearer " || auth[:7] == "bearer ") {
		key = auth[7:]
	}
	if key == "" {
		key = string(ctx.Request.Header.Peek("X-API-Key"))
	}
	return key
}

func ValidateUserID(userID string) error {
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}
	if len(userID) > 100 {
		return fmt.Errorf("user ID too long")
	}
	return nil
}
