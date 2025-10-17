package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/config"
	"progressdb/pkg/logger"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/utils"
)

// caller role
type Role int

const (
	RoleUnauth Role = iota
	RoleFrontend
	RoleBackend
	RoleAdmin
)

// security config
type SecConfig struct {
	AllowedOrigins []string
	RPS            float64
	Burst          int
	IPWhitelist    []string
	BackendKeys    map[string]struct{}
	FrontendKeys   map[string]struct{}
	AdminKeys      map[string]struct{}
}

// require signed hmac
func RequireSignedAuthorFast(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tr := telemetry.Track("auth.require_signed_author")
		defer tr.Finish()

		// parse possible identifiers
		role := strings.ToLower(string(ctx.Request.Header.Peek("X-Role-Name")))
		userID := strings.TrimSpace(string(ctx.Request.Header.Peek("X-User-ID")))
		sig := strings.TrimSpace(string(ctx.Request.Header.Peek("X-User-Signature")))

		// allow no_role reqs - for allowed root middleware routes
		if role != "frontend" && role != "backend" && role != "admin" {
			next(ctx)
			return
		}

		// allow backend reqs - with no signature
		if role == "backend" && sig == "" {
			next(ctx)
			return
		}

		// allow admin reqs - enforce /admin route prefixes
		if role == "admin" && strings.HasPrefix(string(ctx.Path()), "/admin") && sig == "" {
			next(ctx)
			return
		}

		// reject all reqs - if no signature and user details
		if sig == "" || userID == "" {
			logger.Warn("missing_signature_headers", "path", string(ctx.Path()), "remote", ctx.RemoteAddr().String())
			utils.JSONErrorFast(ctx, fasthttp.StatusUnauthorized, "missing signature headers")
			return
		}

		// reject if no signing keys
		keys := config.GetSigningKeys()
		if len(keys) == 0 {
			logger.Error("no_signing_keys_configured")
			utils.JSONErrorFast(ctx, fasthttp.StatusInternalServerError, "server misconfigured: no signing secrets available")
			return
		}

		tr.Mark("verify_signature")
		// crypto verify the req: user_id <> hmac is not tampered
		ok := false
		for k := range keys {
			mac := hmac.New(sha256.New, []byte(k))
			mac.Write([]byte(userID))
			expected := hex.EncodeToString(mac.Sum(nil))
			if hmac.Equal([]byte(expected), []byte(sig)) {
				ok = true
				break
			}
		}
		if !ok {
			logger.Warn("invalid_signature", "user", userID, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			utils.JSONErrorFast(ctx, fasthttp.StatusUnauthorized, "invalid signature")
			return
		}

		// allow req to continue
		logger.Info("signature_verified", "user", userID, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
		ctx.SetUserValue("author", userID)
		next(ctx)
	}
}

func validateAuthor(a string) (bool, string) {
	if a == "" {
		return false, "author required"
	}
	if len(a) > 128 {
		return false, "author too long"
	}
	return true, ""
}

// extract author - depending on frontend or backend role
func ResolveAuthorFromRequestFast(ctx *fasthttp.RequestCtx, bodyAuthor string) (string, int, string) {
	tr := telemetry.Track("auth.resolve_author")
	defer tr.Finish()

	// signature-verified author from user value if present
	if v := ctx.UserValue("author"); v != nil {
		if id, ok := v.(string); ok && id != "" {
			logger.Info("author_signature_verified", "author", id, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			if q := string(ctx.QueryArgs().Peek("author")); q != "" && q != id {
				logger.Warn("author_mismatch_signature_query", "signature", id, "query", q, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", fasthttp.StatusForbidden, "author mismatch between signature and query param"
			}
			if h := string(ctx.Request.Header.Peek("X-User-ID")); h != "" && h != id {
				logger.Warn("author_mismatch_signature_header", "signature", id, "header", h, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", fasthttp.StatusForbidden, "author mismatch between signature and header"
			}
			if bodyAuthor != "" && bodyAuthor != id {
				logger.Warn("author_mismatch_signature_body", "signature", id, "body", bodyAuthor, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", fasthttp.StatusForbidden, "author mismatch between signature and body author"
			}
			logger.Info("author_resolved_signature", "author", id, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			return id, 0, ""
		}
	}

	// no signature; allow backend to supply author via body/header/query
	role := string(ctx.Request.Header.Peek("X-Role-Name"))
	logger.Info("no_signature_found", "role", role, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
	if role == "backend" {
		if bodyAuthor != "" {
			if ok, msg := validateAuthor(bodyAuthor); !ok {
				logger.Warn("invalid_backend_author", "user", bodyAuthor, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", fasthttp.StatusBadRequest, msg
			}
			logger.Info("author_from_body_backend", "user", bodyAuthor, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			return bodyAuthor, 0, ""
		}
		if h := string(ctx.Request.Header.Peek("X-User-ID")); h != "" {
			if ok, msg := validateAuthor(h); !ok {
				logger.Warn("invalid_backend_author", "user", h, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", fasthttp.StatusBadRequest, msg
			}
			logger.Info("author_from_header_backend", "user", h, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			return h, 0, ""
		}
		if q := string(ctx.QueryArgs().Peek("author")); q != "" {
			if ok, msg := validateAuthor(q); !ok {
				logger.Warn("invalid_backend_author", "user", q, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", fasthttp.StatusBadRequest, msg
			}
			logger.Info("author_from_query_backend", "user", q, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			return q, 0, ""
		}
		logger.Warn("backend_missing_author", "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
		return "", fasthttp.StatusBadRequest, "author required for backend requests"
	}

	// otherwise require signature
	logger.Warn("missing_author_signature", "role", role, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
	return "", fasthttp.StatusUnauthorized, "missing or invalid author signature"
}
