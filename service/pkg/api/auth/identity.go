package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/api/utils"
	"progressdb/pkg/config"
	"progressdb/pkg/state/logger"
)

type Role int

const (
	RoleUnauth Role = iota
	RoleFrontend
	RoleBackend
	RoleAdmin
)

func CreateHMACSignature(userID, key string) (string, error) {
	if userID == "" || key == "" {
		return "", &router.AuthorResolutionError{
			Type:    "invalid_input",
			Message: "userID and key are required to create signature",
			Code:    fasthttp.StatusBadRequest,
		}
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(userID))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func VerifyHMACSignature(userID, signature string) bool {
	keys := config.GetSigningKeys()

	if userID == "" || signature == "" {
		return false
	}

	for k := range keys {
		expected, err := CreateHMACSignature(userID, k)
		if err != nil {
			continue
		}
		if hmac.Equal([]byte(expected), []byte(signature)) {
			return true
		}
	}
	return false
}

type SecConfig struct {
	AllowedOrigins []string
	RPS            float64
	Burst          int
	IPWhitelist    []string
	BackendKeys    map[string]struct{}
	FrontendKeys   map[string]struct{}
	AdminKeys      map[string]struct{}
}

func RequireSignedAuthorMiddleware(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		path := utils.GetPath(ctx)
		remote := ctx.RemoteAddr().String()
		logMeta := []interface{}{"path", path, "remote", remote}

		// parse identifiers
		userID := utils.GetUserID(ctx)
		sig := utils.GetUserSignature(ctx)

		// frontend signature verification only
		if sig == "" {
			logger.Warn("missing_signature", logMeta...)
			router.WriteJSONError(ctx, fasthttp.StatusUnauthorized, "missing signature")
			return
		}
		if userID == "" {
			logger.Warn("missing_user_id", logMeta...)
			router.WriteJSONError(ctx, fasthttp.StatusUnauthorized, "missing user id")
			return
		}

		// crypto verify the request: user_id <> hmac is not tampered
		if VerifyHMACSignature(userID, sig) == false {
			logger.Warn("invalid_signature", logMeta...)
			router.WriteJSONError(ctx, fasthttp.StatusUnauthorized, "invalid signature")
			return
		}

		// signature verified - continue
		logger.Info("signature_verified", logMeta...)
		ctx.SetUserValue("author", userID)
		ctx.Request.Header.Set("X-User-ID", userID)
		next(ctx)
	}
}
