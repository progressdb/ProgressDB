package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"progressdb/pkg/config"
	"progressdb/pkg/logger"
	"progressdb/pkg/utils"
)

// Role represents the resolved caller role for a request.
type Role int

const (
	RoleUnauth Role = iota
	RoleFrontend
	RoleBackend
	RoleAdmin
)

// SecConfig mirrors the security-related configuration used to drive
// authentication, CORS and rate limiting behavior. Put here so limiter.go
// and gateway.go can reference the shared type.
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

// RequireSignedAuthor verifies HMAC signature headers and injects the
// verified author id into the request context.
func RequireSignedAuthor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Determine caller role set earlier by gateway middleware
		role := r.Header.Get("X-Role-Name")
		userID := strings.TrimSpace(r.Header.Get("X-User-ID"))
		sig := strings.TrimSpace(r.Header.Get("X-User-Signature"))

		// Backend/admin callers: allow missing signature entirely, or accept
		// a header-provided author without a signature. If a signature is
		// present we will verify it below.
		if role == "backend" || role == "admin" {
			if sig == "" {
				// No signature provided; allow the request through. Handlers may
				// accept an author from body or X-User-ID header as appropriate.
				next.ServeHTTP(w, r)
				return
			}
			// signature present -> fallthrough to verification logic
		}

		// If we reach here and there's no signature, the caller is not a
		// trusted backend/admin and we must require signature headers.
		if sig == "" {
			logger.Warn("missing_signature_headers", "path", r.URL.Path, "remote", r.RemoteAddr)
			utils.JSONError(w, http.StatusUnauthorized, "missing signature headers")
			return
		}
		// signature is present; require userID as well
		if userID == "" {
			logger.Warn("missing_signature_headers", "path", r.URL.Path, "remote", r.RemoteAddr)
			utils.JSONError(w, http.StatusUnauthorized, "missing signature headers")
			return
		}

		// Retrieve signing keys from the canonical config package.
		keys := config.GetSigningKeys()
		if len(keys) == 0 {
			logger.Error("no_signing_keys_configured")
			utils.JSONError(w, http.StatusInternalServerError, "server misconfigured: no signing secrets available")
			return
		}

		// Try all configured signing keys.
		ok := false
		for k := range config.GetSigningKeys() {
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
		// do not set headers; handlers should use context via AuthorIDFromContext
		next.ServeHTTP(w, r)
	})
}

// AuthorIDFromContext returns the verified author id or empty string.
func AuthorIDFromContext(ctx context.Context) string {
	if v := ctx.Value(ctxAuthorKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Note: authenticate and frontendAllowed are implemented in gateway.go
// to keep gateway responsibilities self-contained.

func validateAuthor(a string) (bool, string) {
	if a == "" {
		return false, "author required"
	}
	if len(a) > 128 {
		return false, "author too long"
	}
	return true, ""
}

// ResolveAuthorFromRequest is the single canonical resolver handlers should call.
// It prefers a signature-verified author (in context). If a signature is present
// it is authoritative — any conflicting author provided via header/body/query
// will cause a 403. When no signature is present, backend/admin roles may
// supply an author via body, header (X-User-ID) or query (fallback). Frontend
// callers require a signature and will receive 401 when missing.
func ResolveAuthorFromRequest(r *http.Request, bodyAuthor string) (string, int, string) {
	// Prefer signature-verified author from context
	if id := AuthorIDFromContext(r.Context()); id != "" {
		logger.Info("author_signature_verified", "author", id, "remote", r.RemoteAddr, "path", r.URL.Path)
		// If other provided authors conflict with the signature, reject.
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

	// No signature; allow backend/admins to supply an author via body/header/query.
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

	// Otherwise require signature
	logger.Warn("missing_author_signature", "role", role, "remote", r.RemoteAddr, "path", r.URL.Path)
	return "", http.StatusUnauthorized, "missing or invalid author signature"
}
