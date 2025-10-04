package auth

import (
	"net/http"
	"progressdb/pkg/logger"
	"strings"
)

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
