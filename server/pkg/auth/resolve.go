package auth

import (
    "log/slog"
    "net/http"
    "strings"
)

// ResolveAuthor centralizes logic for determining the canonical author for a request.
// It prefers a signature-verified author injected into the request context. If missing
// and the caller is a trusted backend/admin role, it accepts an author provided by the
// request body (bodyAuthor) or the X-User-ID header. Returns (author, 0, "") on
// success or ("", status, jsonError) on failure.
func ResolveAuthor(r *http.Request, bodyAuthor string) (string, int, string) {
    // First, prefer signature-verified author from context
    if id := AuthorIDFromContext(r.Context()); id != "" {
        return id, 0, ""
    }
    role := r.Header.Get("X-Role-Name")
    // Trusted backends may supply an author in body or header
    if role == "backend" || role == "admin" {
        if bodyAuthor != "" {
            if ok, msg := validateAuthor(bodyAuthor); !ok {
                slog.Warn("invalid_backend_author", "user", bodyAuthor, "remote", r.RemoteAddr, "path", r.URL.Path)
                return "", http.StatusBadRequest, msg
            }
            slog.Info("author_from_body_backend", "user", bodyAuthor, "remote", r.RemoteAddr, "path", r.URL.Path)
            return bodyAuthor, 0, ""
        }
        if h := strings.TrimSpace(r.Header.Get("X-User-ID")); h != "" {
            if ok, msg := validateAuthor(h); !ok {
                slog.Warn("invalid_backend_author", "user", h, "remote", r.RemoteAddr, "path", r.URL.Path)
                return "", http.StatusBadRequest, msg
            }
            slog.Info("author_from_header_backend", "user", h, "remote", r.RemoteAddr, "path", r.URL.Path)
            return h, 0, ""
        }
        slog.Warn("backend_missing_author", "remote", r.RemoteAddr, "path", r.URL.Path)
        return "", http.StatusBadRequest, `{"error":"author required for backend requests"}`
    }
    // Otherwise, require signature-verified author (frontend/unauth)
    return "", http.StatusUnauthorized, `{"error":"missing or invalid author signature"}`
}

func validateAuthor(a string) (bool, string) {
    if a == "" {
        return false, `{"error":"author required"}`
    }
    if len(a) > 128 {
        return false, `{"error":"author too long"}`
    }
    return true, ""
}

