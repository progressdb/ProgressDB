package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/config"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/state/telemetry"
)

// caller role
type Role int

const (
	RoleUnauth Role = iota
	RoleFrontend
	RoleBackend
	RoleAdmin
)

// AuthorResolutionError represents different types of author resolution failures
type AuthorResolutionError struct {
	Type    string
	Message string
	Code    int
}

func (e *AuthorResolutionError) Error() string {
	return e.Message
}

// Predefined author resolution errors
var (
	ErrAuthorRequired     = &AuthorResolutionError{"author_required", "author required", fasthttp.StatusBadRequest}
	ErrAuthorTooLong      = &AuthorResolutionError{"author_too_long", "author too long", fasthttp.StatusBadRequest}
	ErrInvalidSignature   = &AuthorResolutionError{"invalid_signature", "missing or invalid author signature", fasthttp.StatusUnauthorized}
	ErrAuthorMismatch     = &AuthorResolutionError{"author_mismatch", "author mismatch", fasthttp.StatusForbidden}
	ErrBackendMissingAuth = &AuthorResolutionError{"backend_missing_auth", "author required for backend requests", fasthttp.StatusBadRequest}
)

// creates an HMAC signature for a user ID
func CreateHMACSignature(userID, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(userID))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifies a user ID against its HMAC signature using available signing keys
func VerifyHMACSignature(userID, signature string) bool {
	keys := config.GetSigningKeys()

	for k := range keys {
		expected := CreateHMACSignature(userID, k)
		if hmac.Equal([]byte(expected), []byte(signature)) {
			return true
		}
	}
	return false
}

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
			router.WriteJSONError(ctx, fasthttp.StatusUnauthorized, "missing signature headers")
			return
		}

		tr.Mark("verify_signature")

		// crypto verify the req: user_id <> hmac is not tampered
		if !VerifyHMACSignature(userID, sig) {
			logger.Warn("invalid_signature", "user", userID, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			router.WriteJSONError(ctx, fasthttp.StatusUnauthorized, "invalid signature")
			return
		}

		// allow req to continue
		logger.Info("signature_verified", "user", userID, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
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
	tr := telemetry.Track("auth.resolve_author")
	defer tr.Finish()

	// signature-verified author from user value if present
	if v := ctx.UserValue("author"); v != nil {
		if id, ok := v.(string); ok && id != "" {
			logger.Info("author_signature_verified", "author", id, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			if q := string(ctx.QueryArgs().Peek("author")); q != "" && q != id {
				logger.Warn("author_mismatch_signature_query", "signature", id, "query", q, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", &AuthorResolutionError{Type: "author_mismatch", Message: "author mismatch between signature and query param", Code: fasthttp.StatusForbidden}
			}
			if h := string(ctx.Request.Header.Peek("X-User-ID")); h != "" && h != id {
				logger.Warn("author_mismatch_signature_header", "signature", id, "header", h, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", &AuthorResolutionError{Type: "author_mismatch", Message: "author mismatch between signature and header", Code: fasthttp.StatusForbidden}
			}
			if bodyAuthor != "" && bodyAuthor != id {
				logger.Warn("author_mismatch_signature_body", "signature", id, "body", bodyAuthor, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", &AuthorResolutionError{Type: "author_mismatch", Message: "author mismatch between signature and body author", Code: fasthttp.StatusForbidden}
			}
			logger.Info("author_resolved_signature", "author", id, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			ctx.Request.Header.Set("X-User-ID", id)
			return id, nil
		}
	}

	// no signature; allow backend to supply author via body/header/query
	role := string(ctx.Request.Header.Peek("X-Role-Name"))
	logger.Info("no_signature_found", "role", role, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
	if role == "backend" {
		if bodyAuthor != "" {
			if err := validateAuthor(bodyAuthor); err != nil {
				logger.Warn("invalid_backend_author", "user", bodyAuthor, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", err
			}
			logger.Info("author_from_body_backend", "user", bodyAuthor, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			ctx.Request.Header.Set("X-User-ID", bodyAuthor)
			return bodyAuthor, nil
		}
		if h := string(ctx.Request.Header.Peek("X-User-ID")); h != "" {
			if err := validateAuthor(h); err != nil {
				logger.Warn("invalid_backend_author", "user", h, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", err
			}
			logger.Info("author_from_header_backend", "user", h, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			ctx.Request.Header.Set("X-User-ID", h)
			return h, nil
		}
		if q := string(ctx.QueryArgs().Peek("author")); q != "" {
			if err := validateAuthor(q); err != nil {
				logger.Warn("invalid_backend_author", "user", q, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
				return "", err
			}
			logger.Info("author_from_query_backend", "user", q, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			ctx.Request.Header.Set("X-User-ID", q)
			return q, nil
		}
		logger.Warn("backend_missing_author", "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
		return "", ErrBackendMissingAuth
	}

	// otherwise require signature
	logger.Warn("missing_author_signature", "role", role, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
	return "", ErrInvalidSignature
}
