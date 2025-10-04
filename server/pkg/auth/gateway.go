package auth

import (
	"net"
	"net/http"
	"strings"

	"progressdb/pkg/logger"
	"progressdb/pkg/utils"
)

// Role and SecConfig types are defined in identity.go

func AuthenticateRequestMiddleware(cfg SecConfig) func(http.Handler) http.Handler {
	// Rate limiters keyed by API key or remote IP
	limiters := &limiterPool{cfg: cfg}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// request logging (redacts sensitive headers)
			logger.LogRequest(r)
			// CORS preflight
			origin := r.Header.Get("Origin")
			if origin != "" && originAllowed(origin, cfg.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				// Allow common methods used by the API (including mutation methods).
				w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,PATCH,OPTIONS")
				// Cache preflight response for 10 minutes to reduce preflight traffic.
				w.Header().Set("Access-Control-Max-Age", "600")
				// Include common custom headers used by the SDKs (X-API-Key) and
				// the signed-author flow (X-User-ID, X-User-Signature). Keep this
				// list in sync with any client headers you expect to receive.
				w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-API-Key,X-User-ID,X-User-Signature")
				// Expose role header to clients if they need it
				w.Header().Set("Access-Control-Expose-Headers", "X-Role-Name")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// IP whitelist
			if len(cfg.IPWhitelist) > 0 {
				ip := clientIP(r)
				logger.Debug("ip_check", "ip", ip)
				if !ipWhitelisted(ip, cfg.IPWhitelist) {
					utils.JSONError(w, http.StatusForbidden, "forbidden")
					logger.Warn("request_blocked", "reason", "ip_not_whitelisted", "ip", ip, "path", r.URL.Path)
					return
				}
			}

			// Auth
			role, key, hasAPIKey := authenticate(r, cfg)

			// Log authentication outcome (do not log full key content)
			logger.Debug("auth_check", "role", role, "has_api_key", hasAPIKey)

			// Allow unauthenticated health checks for deployment probes.
			// Probes often cannot send API keys; accept GET /healthz without
			// authentication so external systems can verify service liveness.
			if (r.URL.Path == "/healthz" || r.URL.Path == "/readyz") && r.Method == http.MethodGet {
				r.Header.Set("X-Role-Name", "unauth")
				next.ServeHTTP(w, r)
				return
			}

			// Do not allow unauthenticated requests for other endpoints unless
			// the request carries signature headers (X-User-ID + X-User-Signature),
			// in which case signature verification middleware will handle auth.
			if role == RoleUnauth {
				if !(r.Header.Get("X-User-ID") != "" && r.Header.Get("X-User-Signature") != "") {
					utils.JSONError(w, http.StatusUnauthorized, "unauthorized")
					logger.Warn("request_unauthorized", "path", r.URL.Path, "remote", r.RemoteAddr)
					return
				}
				// otherwise, allow through so signature middleware can verify
			}

			// Expose role name for handlers
			var roleName string
			switch role {
			case RoleFrontend:
				roleName = "frontend"
			case RoleBackend:
				roleName = "backend"
			case RoleAdmin:
				roleName = "admin"
			default:
				roleName = "unauth"
			}
			r.Header.Set("X-Role-Name", roleName)

			// Scope enforcement for frontend keys
			if role == RoleFrontend && !frontendAllowed(r) {
				utils.JSONError(w, http.StatusForbidden, "forbidden")
				logger.Warn("request_forbidden", "reason", "frontend_not_allowed", "path", r.URL.Path)
				return
			}

			// Rate limiting
			if !limiters.Allow(key) {
				utils.JSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
				logger.Warn("rate_limited", "has_api_key", hasAPIKey, "path", r.URL.Path)
				return
			}

			// Log that request passed middleware checks
			logger.Info("request_allowed", "method", r.Method, "path", r.URL.Path, "role", r.Header.Get("X-Role-Name"))

			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	for _, a := range allowed {
		if a == "*" || strings.EqualFold(a, origin) {
			return true
		}
	}
	return false
}

// authenticate and frontendAllowed are implemented further below in this file.

func clientIP(r *http.Request) string {
	// Expect direct connection for MVP; ignore X-Forwarded-For
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func ipWhitelisted(ip string, list []string) bool {
	for _, w := range list {
		if ip == w {
			return true
		}
	}
	return false
}

func authenticate(r *http.Request, cfg SecConfig) (Role, string, bool) {
	// Prefer Authorization: Bearer <key>, fallback to X-API-Key
	auth := r.Header.Get("Authorization")
	var key string
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		key = strings.TrimSpace(auth[7:])
	}
	if key == "" {
		key = r.Header.Get("X-API-Key")
	}
	if key == "" {
		// no API key present: return unauth role with client IP as identifier,
		// and signal that no API key was provided (hasAPIKey=false)
		return RoleUnauth, clientIP(r), false
	}
	if cfg.AdminKeys != nil {
		if _, ok := cfg.AdminKeys[key]; ok {
			return RoleAdmin, key, true
		}
	}
	if _, ok := cfg.BackendKeys[key]; ok {
		return RoleBackend, key, true
	}
	if _, ok := cfg.FrontendKeys[key]; ok {
		return RoleFrontend, key, true
	}
	return RoleUnauth, key, true
}

func frontendAllowed(r *http.Request) bool {
	// Allow message create/list
	if r.URL.Path == "/v1/messages" && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
		return true
	}
	// Allow thread collection and thread-scoped APIs for frontend keys.
	// Handlers themselves require a verified author (RequireSignedAuthor)
	// and perform ownership checks where appropriate.
	if strings.HasPrefix(r.URL.Path, "/v1/threads") {
		return true
	}
	return false
}
