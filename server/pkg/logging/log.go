package logging

import (
	"net/http"
	"strings"

	"log/slog"
)

var sensitive = map[string]struct{}{
	"authorization":    {},
	"x-api-key":        {},
	"x-user-signature": {},
}

// redactHeaderValue redacts known sensitive header values.
func redactHeaderValue(k, v string) string {
	if v == "" {
		return ""
	}
	if _, ok := sensitive[strings.ToLower(k)]; ok {
		return "<redacted>"
	}
	return v
}

// SafeHeaders returns a map of headers suitable for logging with sensitive
// values redacted. Only first value is returned for brevity.
func SafeHeaders(r *http.Request) map[string]string {
	out := make(map[string]string)
	for k, v := range r.Header {
		if len(v) == 0 {
			continue
		}
		out[k] = redactHeaderValue(k, v[0])
	}
	return out
}

// LogRequest logs a concise, safe summary of an incoming request.
func LogRequest(r *http.Request) {
	slog.Info("incoming_request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr, "headers", SafeHeaders(r))
}
