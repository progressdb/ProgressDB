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

type AuthorResolutionError struct {
	Type    string
	Message string
	Code    int
}

func (e *AuthorResolutionError) Error() string {
	return e.Message
}

var (
	ErrAuthorRequired     = &AuthorResolutionError{"author_required", "author required", fasthttp.StatusBadRequest}
	ErrAuthorTooLong      = &AuthorResolutionError{"author_too_long", "author too long", fasthttp.StatusBadRequest}
	ErrInvalidSignature   = &AuthorResolutionError{"invalid_signature", "missing or invalid author signature", fasthttp.StatusUnauthorized}
	ErrAuthorMismatch     = &AuthorResolutionError{"author_mismatch", "author mismatch", fasthttp.StatusForbidden}
	ErrBackendMissingAuth = &AuthorResolutionError{"backend_missing_auth", "author required for backend requests", fasthttp.StatusBadRequest}
)

func CreateHMACSignature(userID, key string) (string, error) {
	if userID == "" || key == "" {
		return "", &AuthorResolutionError{
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

		// parse possible identifiers
		role := utils.GetRole(ctx)
		userID := utils.GetUserID(ctx)
		sig := utils.GetUserSignature(ctx)

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
		if role == "admin" && utils.HasPathPrefix(ctx, "/admin") && sig == "" {
			next(ctx)
			return
		}

		// allow backend reqs - with no signature
		if role == "backend" && sig == "" {
			next(ctx)
			return
		}

		// allow admin reqs
		if role == "admin" && sig == "" {
			next(ctx)
			return
		}

		// frontend - requires sig and userid
		if role == "frontend" {
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

			// crypto verify the req: user_id <> hmac is not tampered
			if VerifyHMACSignature(userID, sig) == false {
				logger.Warn("invalid_signature", logMeta...)
				router.WriteJSONError(ctx, fasthttp.StatusUnauthorized, "invalid signature")
				return
			}
		}

		// allow req to continue
		logger.Info("signature_verified", logMeta...)
		ctx.SetUserValue("author", userID)
		ctx.Request.Header.Set("X-User-ID", userID)
		next(ctx)
	}
}

func validateAuthor(a string) *AuthorResolutionError {
	if a == "" {
		return ErrAuthorRequired
	}
	if len(a) > 128 {
		return ErrAuthorTooLong
	}
	return nil
}

// extract author - depending on frontend or backend role
func ResolveAuthorFromRequestFast(ctx *fasthttp.RequestCtx, bodyAuthor string) (string, *AuthorResolutionError) {
	// signature-verified author from user value if present
	if v := ctx.UserValue("author"); v != nil {
		if id, ok := v.(string); ok && id != "" {
			logger.Info("author_signature_verified", "author", id, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
			if q := utils.GetQuery(ctx, "author"); q != "" && q != id {
				logger.Warn("author_mismatch_signature_query", "signature", id, "query", q, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
				return "", &AuthorResolutionError{Type: "author_mismatch", Message: "author mismatch between signature and query param", Code: fasthttp.StatusForbidden}
			}
			if h := utils.GetUserID(ctx); h != "" && h != id {
				logger.Warn("author_mismatch_signature_header", "signature", id, "header", h, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
				return "", &AuthorResolutionError{Type: "author_mismatch", Message: "author mismatch between signature and header", Code: fasthttp.StatusForbidden}
			}
			if bodyAuthor != "" && bodyAuthor != id {
				logger.Warn("author_mismatch_signature_body", "signature", id, "body", bodyAuthor, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
				return "", &AuthorResolutionError{Type: "author_mismatch", Message: "author mismatch between signature and body author", Code: fasthttp.StatusForbidden}
			}
			logger.Info("author_resolved_signature", "author", id, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
			ctx.Request.Header.Set("X-User-ID", id)
			return id, nil
		}
	}

	// no signature; allow backend to supply author via body/header/query
	role := utils.GetRole(ctx)
	logger.Info("no_signature_found", "role", role, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
	if role == "backend" {
		if bodyAuthor != "" {
			if err := validateAuthor(bodyAuthor); err != nil {
				logger.Warn("invalid_backend_author", "user", bodyAuthor, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
				return "", err
			}
			logger.Info("author_from_body_backend", "user", bodyAuthor, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
			ctx.Request.Header.Set("X-User-ID", bodyAuthor)
			return bodyAuthor, nil
		}
		if h := utils.GetUserID(ctx); h != "" {
			if err := validateAuthor(h); err != nil {
				logger.Warn("invalid_backend_author", "user", h, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
				return "", err
			}
			logger.Info("author_from_header_backend", "user", h, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
			ctx.Request.Header.Set("X-User-ID", h)
			return h, nil
		}
		if q := utils.GetQuery(ctx, "author"); q != "" {
			if err := validateAuthor(q); err != nil {
				logger.Warn("invalid_backend_author", "user", q, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
				return "", err
			}
			logger.Info("author_from_query_backend", "user", q, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
			ctx.Request.Header.Set("X-User-ID", q)
			return q, nil
		}
		logger.Warn("backend_missing_author", "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
		return "", ErrBackendMissingAuth
	}

	// otherwise require signature
	logger.Warn("missing_author_signature", "role", role, "remote", ctx.RemoteAddr().String(), "path", utils.GetPath(ctx))
	return "", ErrInvalidSignature
}
