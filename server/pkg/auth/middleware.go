package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"net/http"
	"progressdb/pkg/logger"
	"strings"

	"progressdb/pkg/config"
)

type ctxAuthorKey struct{}

// RequireSignedAuthor verifies HMAC signature headers and injects the
// verified author id into the request context.
func RequireSignedAuthor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Determine caller role set earlier by security middleware
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
			http.Error(w, `{"error":"missing signature headers"}`, http.StatusUnauthorized)
			return
		}
		// signature is present; require userID as well
		if userID == "" {
			logger.Warn("missing_signature_headers", "path", r.URL.Path, "remote", r.RemoteAddr)
			http.Error(w, `{"error":"missing signature headers"}`, http.StatusUnauthorized)
			return
		}

		// Retrieve signing keys from the canonical config package.
		keys := config.GetSigningKeys()
		if len(keys) == 0 {
			logger.Error("no_signing_keys_configured")
			http.Error(w, `{"error":"server misconfigured: no signing secrets available"}`, http.StatusInternalServerError)
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
			http.Error(w, `{"error":"invalid signature"}`, http.StatusUnauthorized)
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
