package auth

import (
	"net"
	"strings"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/logger"
)

func AuthenticateRequestMiddlewareFast(cfg SecConfig) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	limiters := &limiterPool{cfg: cfg}
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			// cors headers and handle options shortcut
			origin := string(ctx.Request.Header.Peek("Origin"))
			if origin != "" && originAllowed(origin, cfg.AllowedOrigins) {
				ctx.Response.Header.Set("Access-Control-Allow-Origin", origin)
				ctx.Response.Header.Set("Vary", "Origin")
				ctx.Response.Header.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,PATCH,OPTIONS")
				ctx.Response.Header.Set("Vary", "Origin")
				ctx.Response.Header.Set("Access-Control-Max-Age", "600")
				ctx.Response.Header.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-API-Key,X-User-ID,X-User-Signature")
				ctx.Response.Header.Set("Access-Control-Expose-Headers", "X-Role-Name")
			}
			if string(ctx.Method()) == fasthttp.MethodOptions {
				ctx.SetStatusCode(fasthttp.StatusNoContent)
				return
			}

			// ip whitelist check (always before all other checks except cors/options)
			if len(cfg.IPWhitelist) > 0 {
				ip := clientIPFast(ctx)
				logger.Debug("ip_check", "ip", ip)
				if !ipWhitelisted(ip, cfg.IPWhitelist) {
					router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
					logger.Warn("request_blocked", "reason", "ip_not_whitelisted", "ip", ip, "path", string(ctx.Path()))
					return
				}
			}

			// public endpoint check (before auth for health)
			if publicAllowedPath(ctx) {
				ctx.Request.Header.Set("X-Role-Name", "unauth")
				next(ctx)
				return
			}

			// api key validation and role extraction
			role, key, hasAPIKey := validateAPIKey(ctx, cfg)
			logger.Debug("auth_check", "role", role, "has_api_key", hasAPIKey)

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

			// if not an authenticated role, deny request
			if role == RoleUnauth || !hasAPIKey {
				router.WriteJSONError(ctx, fasthttp.StatusUnauthorized, "unauthorized")
				logger.Warn("request_unauthorized", "path", string(ctx.Path()), "remote", ctx.RemoteAddr().String())
				return
			}
			ctx.Request.Header.Set("X-Role-Name", roleName)

			// apply role-based route restrictions
			// frontends can only access thread/messages routes
			if role == RoleFrontend && !frontendAllowedFast(ctx) {
				router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
				logger.Warn("request_forbidden", "reason", "frontend_not_allowed", "path", string(ctx.Path()))
				return
			}
			// backend or frontend cannot access /admin
			if (role == RoleBackend || role == RoleFrontend) && strings.HasPrefix(string(ctx.Path()), "/admin") {
				router.WriteJSONError(ctx, fasthttp.StatusForbidden, "backend api keys cannot access admin routes")
				logger.Warn("backend_admin_access_attempt", "path", string(ctx.Path()), "remote", ctx.RemoteAddr().String())
				return
			}
			// admins can only access /admin paths
			if role == RoleAdmin && !strings.HasPrefix(string(ctx.Path()), "/admin") {
				router.WriteJSONError(ctx, fasthttp.StatusForbidden, "admin api keys may only access /admin routes")
				logger.Warn("admin_route_violation", "path", string(ctx.Path()), "remote", ctx.RemoteAddr().String())
				return
			}

			// rate limiting (per-key)
			if !limiters.Allow(key) {
				router.WriteJSONError(ctx, fasthttp.StatusTooManyRequests, "rate limit exceeded")
				logger.Warn("rate_limited", "has_api_key", hasAPIKey, "path", string(ctx.Path()))
				return
			}

			// signed author logic (for frontend/client/user-specific requests)
			if string(ctx.Request.Header.Peek("X-User-Signature")) != "" {
				RequireSignedAuthorFast(next)(ctx)
				return
			}

			// authorized: continue to handler
			next(ctx)
		}
	}
}

func clientIPFast(ctx *fasthttp.RequestCtx) string {
	host := ctx.RemoteAddr().String()
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		return host
	}
	return h
}

func validateAPIKey(ctx *fasthttp.RequestCtx, cfg SecConfig) (Role, string, bool) {
	auth := string(ctx.Request.Header.Peek("Authorization"))
	var key string

	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		key = strings.TrimSpace(auth[7:])
	}

	if key == "" {
		key = string(ctx.Request.Header.Peek("X-API-Key"))
	}

	if key == "" {
		return RoleUnauth, clientIPFast(ctx), false
	}

	if cfg.AdminKeys != nil {
		if _, ok := cfg.AdminKeys[key]; ok {
			return RoleAdmin, key, true
		}
	}
	if cfg.BackendKeys != nil {
		if _, ok := cfg.BackendKeys[key]; ok {
			return RoleBackend, key, true
		}
	}
	if cfg.FrontendKeys != nil {
		if _, ok := cfg.FrontendKeys[key]; ok {
			return RoleFrontend, key, true
		}
	}
	return RoleUnauth, key, true
}

func frontendAllowedFast(ctx *fasthttp.RequestCtx) bool {
	path := string(ctx.Path())
	if strings.HasPrefix(path, "/v1/messages") {
		return true
	}
	if strings.HasPrefix(path, "/v1/threads") {
		return true
	}
	return false
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

func ipWhitelisted(ip string, list []string) bool {
	for _, w := range list {
		if ip == w {
			return true
		}
	}
	return false
}

func publicAllowedPath(ctx *fasthttp.RequestCtx) bool {
	path := string(ctx.Path())
	method := string(ctx.Method())

	if (path == "/healthz" || path == "/readyz") && method == fasthttp.MethodGet {
		return true
	}

	return false
}
