package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/valyala/fasthttp"
	"progressdb/pkg/logger"
	"progressdb/pkg/router"
	"progressdb/pkg/utils"
)

// RegisterSigningFast registers the fasthttp-native signing endpoint.
func RegisterSigningFast(r *router.Router) {
	r.POST("/_sign", signHandlerFast)
	r.POST("/v1/_sign", signHandlerFast)
}

func signHandlerFast(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	logger.Info("signHandler called", "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))

	role := string(ctx.Request.Header.Peek("X-Role-Name"))
	if role != "backend" {
		logger.Warn("forbidden: non-backend role attempted to sign", "role", role, "remote", ctx.RemoteAddr().String())
		utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "forbidden")
		return
	}

	auth := string(ctx.Request.Header.Peek("Authorization"))
	var key string
	if len(auth) > 7 && (auth[:7] == "Bearer " || auth[:7] == "bearer ") {
		key = auth[7:]
	}
	if key == "" {
		key = string(ctx.Request.Header.Peek("X-API-Key"))
	}
	if key == "" {
		logger.Warn("missing api key in signHandler", "remote", ctx.RemoteAddr().String())
		utils.JSONErrorFast(ctx, fasthttp.StatusUnauthorized, "missing api key")
		return
	}

	var payload struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&payload); err != nil || payload.UserID == "" {
		logger.Warn("invalid payload in signHandler", "error", err, "remote", ctx.RemoteAddr().String())
		utils.JSONErrorFast(ctx, fasthttp.StatusBadRequest, "invalid payload")
		return
	}

	logger.Info("signing userId", "remote", ctx.RemoteAddr().String(), "role", role)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload.UserID))
	sig := hex.EncodeToString(mac.Sum(nil))
	if err := json.NewEncoder(ctx).Encode(map[string]string{"userId": payload.UserID, "signature": sig}); err != nil {
		logger.Error("failed to encode signHandler response", "error", err, "remote", ctx.RemoteAddr().String())
	}
}
