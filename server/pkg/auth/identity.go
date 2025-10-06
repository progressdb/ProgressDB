package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/config"
	"progressdb/pkg/logger"
	"progressdb/pkg/utils"
)

// role represents the resolved caller role for a request
type Role int

const (
	RoleUnauth Role = iota
	RoleFrontend
	RoleBackend
	RoleAdmin
)

// secconfig holds security-related config for authentication, cors, and rate limiting
type SecConfig struct {
	AllowedOrigins []string
	RPS            float64
	Burst          int
	IPWhitelist    []string
	BackendKeys    map[string]struct{}
	FrontendKeys   map[string]struct{}
	AdminKeys      map[string]struct{}
}

type ctxAuthorKey struct{}

// requiresignedauthor checks for hmac signature headers and, if valid, injects the
// verified author id into the request context. for backend/admin, signature is optional.
func RequireSignedAuthor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := r.Header.Get("X-Role-Name")
		userID := strings.TrimSpace(r.Header.Get("X-User-ID"))
		sig := strings.TrimSpace(r.Header.Get("X-User-Signature"))

		// for backend/admin, allow through if no signature is present
		if (role == "backend" || role == "admin") && sig == "" {
			next.ServeHTTP(w, r)
			return
		}

		// for all others, require both signature and userid
		if sig == "" {
			logger.Warn("missing_signature_headers", "path", r.URL.Path, "remote", r.RemoteAddr)
			utils.JSONError(w, http.StatusUnauthorized, "missing signature headers")
			return
		}
		if userID == "" {
			logger.Warn("missing_signature_headers", "path", r.URL.Path, "remote", r.RemoteAddr)
			utils.JSONError(w, http.StatusUnauthorized, "missing signature headers")
			return
		}

		// get signing keys from config
		keys := config.GetSigningKeys()
		if len(keys) == 0 {
			logger.Error("no_signing_keys_configured")
			utils.JSONError(w, http.StatusInternalServerError, "server misconfigured: no signing secrets available")
			return
		}

		// try all configured signing keys
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
			logger.Warn("invalid_signature", "user", userID)
			utils.JSONError(w, http.StatusUnauthorized, "invalid signature")
			return
		}
		logger.Info("signature_verified", "user", userID)
		ctx := context.WithValue(r.Context(), ctxAuthorKey{}, userID)
		r = r.WithContext(ctx)
		// do not set headers; handlers should use context via authoridfromcontext
		next.ServeHTTP(w, r)
	})
}

// authoridfromcontext returns the verified author id or empty string
func AuthorIDFromContext(ctx context.Context) string {
	if v := ctx.Value(ctxAuthorKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// note: authenticate and frontendallowed are implemented in gateway.go

func validateAuthor(a string) (bool, string) {
	if a == "" {
		return false, "author required"
	}
	if len(a) > 128 {
		return false, "author too long"
	}
	return true, ""
}

// resolveauthorfromrequest is the canonical resolver for handlers.
// prefers a signature-verified author (from context). if a signature is present,
// it is authoritative: any conflicting author from header/body/query causes 403.
// when no signature, backend/admin may supply author via body, header, or query.
// frontend callers require a signature and get 401 if missing.
func ResolveAuthorFromRequest(r *http.Request, bodyAuthor string) (string, int, string) {
	// prefer signature-verified author from context
	if id := AuthorIDFromContext(r.Context()); id != "" {
		logger.Info("author_signature_verified", "author", id, "remote", r.RemoteAddr, "path", r.URL.Path)
		// reject if any other provided author conflicts with signature
		if q := strings.TrimSpace(r.URL.Query().Get("author")); q != "" && q != id {
			logger.Warn("author_mismatch_signature_query", "signature", id, "query", q, "remote", r.RemoteAddr, "path", r.URL.Path)
			return "", http.StatusForbidden, "author mismatch between signature and query param"
		}
		if h := strings.TrimSpace(r.Header.Get("X-User-ID")); h != "" && h != id {
			logger.Warn("author_mismatch_signature_header", "signature", id, "header", h, "remote", r.RemoteAddr, "path", r.URL.Path)
			return "", http.StatusForbidden, "author mismatch between signature and header"
		}
		if bodyAuthor != "" && bodyAuthor != id {
			logger.Warn("author_mismatch_signature_body", "signature", id, "body", bodyAuthor, "remote", r.RemoteAddr, "path", r.URL.Path)
			return "", http.StatusForbidden, "author mismatch between signature and body author"
		}
		logger.Info("author_resolved_signature", "author", id, "remote", r.RemoteAddr, "path", r.URL.Path)
		return id, 0, ""
	}

	// no signature; allow backend/admin to supply author via body/header/query
	role := r.Header.Get("X-Role-Name")
	logger.Info("no_signature_found", "role", role, "remote", r.RemoteAddr, "path", r.URL.Path)
	if role == "backend" || role == "admin" {
		if bodyAuthor != "" {
			if ok, msg := validateAuthor(bodyAuthor); !ok {
				logger.Warn("invalid_backend_author", "user", bodyAuthor, "remote", r.RemoteAddr, "path", r.URL.Path)
				return "", http.StatusBadRequest, msg
			}
			logger.Info("author_from_body_backend", "user", bodyAuthor, "remote", r.RemoteAddr, "path", r.URL.Path)
			return bodyAuthor, 0, ""
		}
		if h := strings.TrimSpace(r.Header.Get("X-User-ID")); h != "" {
			if ok, msg := validateAuthor(h); !ok {
				logger.Warn("invalid_backend_author", "user", h, "remote", r.RemoteAddr, "path", r.URL.Path)
				return "", http.StatusBadRequest, msg
			}
			logger.Info("author_from_header_backend", "user", h, "remote", r.RemoteAddr, "path", r.URL.Path)
			return h, 0, ""
		}
		if q := strings.TrimSpace(r.URL.Query().Get("author")); q != "" {
			if ok, msg := validateAuthor(q); !ok {
				logger.Warn("invalid_backend_author", "user", q, "remote", r.RemoteAddr, "path", r.URL.Path)
				return "", http.StatusBadRequest, msg
			}
			logger.Info("author_from_query_backend", "user", q, "remote", r.RemoteAddr, "path", r.URL.Path)
			return q, 0, ""
		}
		logger.Warn("backend_missing_author", "remote", r.RemoteAddr, "path", r.URL.Path)
		return "", http.StatusBadRequest, "author required for backend requests"
	}

	// otherwise require signature
	logger.Warn("missing_author_signature", "role", role, "remote", r.RemoteAddr, "path", r.URL.Path)
	return "", http.StatusUnauthorized, "missing or invalid author signature"
}

// ResolveAuthorFromRequestFast is the fasthttp variant of ResolveAuthorFromRequest.
func ResolveAuthorFromRequestFast(ctx *fasthttp.RequestCtx, bodyAuthor string) (string, int, string) {
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

	role := string(ctx.Request.Header.Peek("X-Role-Name"))
	logger.Info("no_signature_found", "role", role, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
	if role == "backend" || role == "admin" {
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

	logger.Warn("missing_author_signature", "role", role, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
	return "", fasthttp.StatusUnauthorized, "missing or invalid author signature"
}
