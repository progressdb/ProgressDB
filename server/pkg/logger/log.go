package logger

import (
    "net/http"
    "strings"
)

var sensitive = map[string]struct{}{
	"authorization":    {},
	"x-api-key":        {},
	"x-user-signature": {},
}

func redactHeaderValue(k, v string) string {
	if v == "" {
		return ""
	}
	if _, ok := sensitive[strings.ToLower(k)]; ok {
		return "<redacted>"
	}
	return v
}

// SafeHeaders returns a compact string representation of headers suitable for
// logging with sensitive values redacted.
func SafeHeaders(r *http.Request) string {
	parts := make([]string, 0, len(r.Header))
	for k, v := range r.Header {
		if len(v) == 0 {
			continue
		}
		parts = append(parts, k+"="+redactHeaderValue(k, v[0]))
	}
	return strings.Join(parts, "; ")
}

// LogRequest logs a concise, safe summary of an incoming request.
func LogRequest(r *http.Request) {
    if Log == nil {
        return
    }
    Info("incoming_request", map[string]interface{}{
        "method":  r.Method,
        "path":    r.URL.Path,
        "remote":  r.RemoteAddr,
        "headers": SafeHeaders(r),
    })
}
