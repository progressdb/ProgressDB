package security

import (
	"net"
	"net/http"
	"strings"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"progressdb/pkg/logger"
)

type Role int

const (
	RoleUnauth Role = iota
	RoleFrontend
	RoleBackend
	RoleAdmin
)

type SecConfig struct {
	AllowedOrigins []string
	RPS            float64
	Burst          int
	IPWhitelist    []string
	BackendKeys    map[string]struct{}
	FrontendKeys   map[string]struct{}
	AdminKeys      map[string]struct{}
}

func AuthenticateRequestMiddleware(cfg SecConfig) func(http.Handler) http.Handler {
	// Rate limiters keyed by API key or remote IP
	limiters := &limiterPool{cfg: cfg}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Centralized safe request logging (redacts sensitive headers)
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
				logger.Log.Debug("ip_check", zap.String("ip", ip))
				if !ipWhitelisted(ip, cfg.IPWhitelist) {
					http.Error(w, "forbidden", http.StatusForbidden)
					logger.Log.Warn("request_blocked", zap.String("reason", "ip_not_whitelisted"), zap.String("ip", ip), zap.String("path", r.URL.Path))

					return
				}
			}

			// Auth
			role, key, hasAPIKey := authenticate(r, cfg)

			// Log authentication outcome (do not log full key content)
			logger.Log.Debug("auth_check", zap.Any("role", role), zap.Bool("has_api_key", hasAPIKey))

			// Allow unauthenticated health checks for deployment probes.
			// Probes often cannot send API keys; accept GET /healthz without
			// authentication so external systems can verify service liveness.
			if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
				r.Header.Set("X-Role-Name", "unauth")
				next.ServeHTTP(w, r)
				return
			}

			// Do not allow unauthenticated requests for other endpoints unless
			// the request carries signature headers (X-User-ID + X-User-Signature),
			// in which case signature verification middleware will handle auth.
			if role == RoleUnauth {
				if !(r.Header.Get("X-User-ID") != "" && r.Header.Get("X-User-Signature") != "") {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					logger.Log.Warn("request_unauthorized", zap.String("path", r.URL.Path), zap.String("remote", r.RemoteAddr))
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
				http.Error(w, "forbidden", http.StatusForbidden)
				logger.Log.Warn("request_forbidden", zap.String("reason", "frontend_not_allowed"), zap.String("path", r.URL.Path))
				return
			}

			// Rate limiting
			if !limiters.Allow(key) {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				logger.Log.Warn("rate_limited", zap.Bool("has_api_key", hasAPIKey), zap.String("path", r.URL.Path))
				return
			}

			// Log that request passed middleware checks
			logger.Log.Info("request_allowed", zap.String("method", r.Method), zap.String("path", r.URL.Path), zap.String("role", r.Header.Get("X-Role-Name")))

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

type limiterPool struct {
	mu  sync.Mutex
	m   map[string]*rate.Limiter
	cfg SecConfig
}

func (p *limiterPool) get(key string) *rate.Limiter {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = make(map[string]*rate.Limiter)
	}
	if l, ok := p.m[key]; ok {
		return l
	}
	rps := p.cfg.RPS
	if rps <= 0 {
		rps = 5
	}
	burst := p.cfg.Burst
	if burst <= 0 {
		burst = 10
	}
	l := rate.NewLimiter(rate.Limit(rps), burst)
	p.m[key] = l
	return l
}

func (p *limiterPool) Allow(key string) bool {
	// Use per-second rate; limiter handles clocks
	return p.get(key).Allow()
}
