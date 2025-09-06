package security

import (
	"net"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"
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
	AllowUnauth    bool
}

func NewMiddleware(cfg SecConfig) func(http.Handler) http.Handler {
	// Rate limiters keyed by API key or remote IP
	limiters := &limiterPool{cfg: cfg}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// CORS preflight
			origin := r.Header.Get("Origin")
			if origin != "" && originAllowed(origin, cfg.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-API-Key")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// IP whitelist
			if len(cfg.IPWhitelist) > 0 {
				ip := clientIP(r)
				if !ipWhitelisted(ip, cfg.IPWhitelist) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}

			// Auth
			role, key := authenticate(r, cfg)
			if role == RoleUnauth && !cfg.AllowUnauth {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
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
				return
			}

			// Rate limiting
			if !limiters.Allow(key) {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

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

func authenticate(r *http.Request, cfg SecConfig) (Role, string) {
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
		return RoleUnauth, clientIP(r)
	}
	if cfg.AdminKeys != nil {
		if _, ok := cfg.AdminKeys[key]; ok {
			return RoleAdmin, key
		}
	}
	if _, ok := cfg.BackendKeys[key]; ok {
		return RoleBackend, key
	}
	if _, ok := cfg.FrontendKeys[key]; ok {
		return RoleFrontend, key
	}
	return RoleUnauth, key
}

func frontendAllowed(r *http.Request) bool {
	// Allow only GET/POST /v1/messages and GET /healthz
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		return true
	}
	if r.URL.Path == "/v1/messages" && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
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
