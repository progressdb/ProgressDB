package auth

import (
	"go.uber.org/zap"
	"net/http"
	"progressdb/pkg/logger"
	"strings"
)

func validateAuthor(a string) (bool, string) {
	if a == "" {
		return false, `{"error":"author required"}`
	}
	if len(a) > 128 {
		return false, `{"error":"author too long"}`
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
		logger.Log.Info("author_signature_verified", zap.String("author", id), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
		// If other provided authors conflict with the signature, reject.
		if q := strings.TrimSpace(r.URL.Query().Get("author")); q != "" && q != id {
			logger.Log.Warn("author_mismatch_signature_query", zap.String("signature", id), zap.String("query", q), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
			return "", http.StatusForbidden, `{"error":"author mismatch between signature and query param"}`
		}
		if h := strings.TrimSpace(r.Header.Get("X-User-ID")); h != "" && h != id {
			logger.Log.Warn("author_mismatch_signature_header", zap.String("signature", id), zap.String("header", h), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
			return "", http.StatusForbidden, `{"error":"author mismatch between signature and header"}`
		}
		if bodyAuthor != "" && bodyAuthor != id {
			logger.Log.Warn("author_mismatch_signature_body", zap.String("signature", id), zap.String("body", bodyAuthor), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
			return "", http.StatusForbidden, `{"error":"author mismatch between signature and body author"}`
		}
		logger.Log.Info("author_resolved_signature", zap.String("author", id), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
		return id, 0, ""
	}

	// No signature; allow backend/admins to supply an author via body/header/query.
	role := r.Header.Get("X-Role-Name")
	logger.Log.Info("no_signature_found", zap.String("role", role), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
	if role == "backend" || role == "admin" {
		if bodyAuthor != "" {
			if ok, msg := validateAuthor(bodyAuthor); !ok {
				logger.Log.Warn("invalid_backend_author", zap.String("user", bodyAuthor), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
				return "", http.StatusBadRequest, msg
			}
			logger.Log.Info("author_from_body_backend", zap.String("user", bodyAuthor), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
			return bodyAuthor, 0, ""
		}
		if h := strings.TrimSpace(r.Header.Get("X-User-ID")); h != "" {
			if ok, msg := validateAuthor(h); !ok {
				logger.Log.Warn("invalid_backend_author", zap.String("user", h), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
				return "", http.StatusBadRequest, msg
			}
			logger.Log.Info("author_from_header_backend", zap.String("user", h), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
			return h, 0, ""
		}
		if q := strings.TrimSpace(r.URL.Query().Get("author")); q != "" {
			if ok, msg := validateAuthor(q); !ok {
				logger.Log.Warn("invalid_backend_author", zap.String("user", q), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
				return "", http.StatusBadRequest, msg
			}
			logger.Log.Info("author_from_query_backend", zap.String("user", q), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
			return q, 0, ""
		}
		logger.Log.Warn("backend_missing_author", zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
		return "", http.StatusBadRequest, `{"error":"author required for backend requests"}`
	}

	// Otherwise require signature
	logger.Log.Warn("missing_author_signature", zap.String("role", role), zap.String("remote", r.RemoteAddr), zap.String("path", r.URL.Path))
	return "", http.StatusUnauthorized, `{"error":"missing or invalid author signature"}`
}
