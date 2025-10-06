package auth

import (
	"net"
	"net/http"
	"strings"

	"progressdb/pkg/logger"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/utils"
)

// role and secconfig types are defined in identity.go

func AuthenticateRequestMiddleware(cfg SecConfig) func(http.Handler) http.Handler {
	// rate limiters keyed by api key or remote ip
	limiters := &limiterPool{cfg: cfg}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// log request (redacts sensitive headers)
			logger.LogRequest(r)

			// cors preflight
			origin := r.Header.Get("Origin")
			if origin != "" && originAllowed(origin, cfg.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,PATCH,OPTIONS")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-API-Key,X-User-ID,X-User-Signature")
				w.Header().Set("Access-Control-Expose-Headers", "X-Role-Name")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// ip whitelist
			if len(cfg.IPWhitelist) > 0 {
				ip := clientIP(r)
				logger.Debug("ip_check", "ip", ip)
				if !ipWhitelisted(ip, cfg.IPWhitelist) {
					utils.JSONError(w, http.StatusForbidden, "forbidden")
					logger.Warn("request_blocked", "reason", "ip_not_whitelisted", "ip", ip, "path", r.URL.Path)
					return
				}
			}

			// extract authentication key from this
			authSpan := telemetry.StartSpan(r.Context(), "auth.authenticate")
			role, key, hasAPIKey := authenticate(r, cfg)
			authSpan()
			logger.Debug("auth_check", "role", role, "has_api_key", hasAPIKey)

			// allow unauthenticated health checks for probes
			if (r.URL.Path == "/healthz" || r.URL.Path == "/readyz") && r.Method == http.MethodGet {
				r.Header.Set("X-Role-Name", "unauth")
				next.ServeHTTP(w, r)
				return
			}

			// expose role name for handlers
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

			// block unauthenticated roles
			if role == RoleUnauth || !hasAPIKey {
				utils.JSONError(w, http.StatusUnauthorized, "unauthorized")
				logger.Warn("request_unauthorized", "path", r.URL.Path, "remote", r.RemoteAddr)
				return
			} else {
				// set role type for downstream
				r.Header.Set("X-Role-Name", roleName)
			}

			// scope enforcement for frontend keys
			if role == RoleFrontend && !frontendAllowed(r) {
				utils.JSONError(w, http.StatusForbidden, "forbidden")
				logger.Warn("request_forbidden", "reason", "frontend_not_allowed", "path", r.URL.Path)
				return
			}

			// rate limiting
			rlSpan := telemetry.StartSpan(r.Context(), "auth.rate_limit")
			if !limiters.Allow(key) {
				rlSpan()
				utils.JSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
				logger.Warn("rate_limited", "has_api_key", hasAPIKey, "path", r.URL.Path)
				return
			}
			rlSpan()

			// log that request passed middleware checks
			logger.Info("request_allowed", "method", r.Method, "path", r.URL.Path, "role", r.Header.Get("X-Role-Name"))

			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowed []string) bool {
	// check if origin is allowed
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

func clientIP(r *http.Request) string {
	// get client ip from remoteaddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func ipWhitelisted(ip string, list []string) bool {
	// check if ip is in whitelist
	for _, w := range list {
		if ip == w {
			return true
		}
	}
	return false
}

func authenticate(r *http.Request, cfg SecConfig) (Role, string, bool) {
	// prefer authorization: bearer <key>, fallback to x-api-key
	auth := r.Header.Get("Authorization")
	var key string
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		key = strings.TrimSpace(auth[7:])
	}
	if key == "" {
		key = r.Header.Get("X-API-Key")
	}
	if key == "" {
		// no api key: return unauth role with client ip, hasapikey=false
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
	// allow message create/list
	if r.URL.Path == "/v1/messages" && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
		return true
	}
	// allow thread collection and thread-scoped apis for frontend keys
	if strings.HasPrefix(r.URL.Path, "/v1/threads") {
		return true
	}
	return false
}
