package logger

import (
	"net/http"
	"strings"
	"unicode/utf8"
)

func maskedValue(v string) string {
	if v == "" {
		return ""
	}
	l := utf8.RuneCountInString(v)
	if l <= 2 {
		return "<redacted>"
	}
	first, _ := utf8.DecodeRuneInString(v)
	last, _ := utf8.DecodeLastRuneInString(v)
	return string(first) + "*****" + string(last)
}

func redactHeaderValue(_ string, v string) string {
	if v == "" {
		return ""
	}
	return maskedValue(v)
}

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

func LogRequest(r *http.Request) {
	if Log == nil {
		return
	}
	Info("incoming_request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr, "headers", SafeHeaders(r))
}
