package backend

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/auth"
	"progressdb/pkg/api/router"
	"progressdb/pkg/config"
	"progressdb/pkg/state/logger"
)

func Sign(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	logger.Info("signHandler called", "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))

	if !isBackendRequest(ctx) {
		logger.Warn("forbidden: non-backend role attempted to sign", "remote", ctx.RemoteAddr().String())
		router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}

	var payload struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&payload); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid JSON payload")
		return
	}

	// Validate user ID format and content
	if err := ValidateUserID(payload.UserID); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, fmt.Sprintf("invalid user ID: %s", err.Error()))
		return
	}

	signingKey, err := getSigningKey()
	if err != nil {
		logger.Error("failed to get signing key", "error", err, "remote", ctx.RemoteAddr().String())
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	sig := auth.CreateHMACSignature(payload.UserID, signingKey)

	if err := router.WriteJSON(ctx, map[string]string{"userId": payload.UserID, "signature": sig}); err != nil {
		logger.Error("failed to encode signHandler response", "error", err, "remote", ctx.RemoteAddr().String())
	}
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

func isBackendRequest(ctx *fasthttp.RequestCtx) bool {
	return string(ctx.Request.Header.Peek("X-Role-Name")) == "backend"
}

func getSigningKey() (string, error) {
	signingKeys := config.GetSigningKeys()
	if len(signingKeys) == 0 {
		return "", fmt.Errorf("signing keys not configured")
	}

	// Return the first available signing key
	for k := range signingKeys {
		return k, nil
	}

	return "", fmt.Errorf("no signing keys available")
}
