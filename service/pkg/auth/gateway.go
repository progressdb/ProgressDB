package auth

import (
	"net"
	"strings"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/logger"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/utils"
)

// entry way for all requests - authenticated & authorized
func AuthenticateRequestMiddlewareFast(cfg SecConfig) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	limiters := &limiterPool{cfg: cfg}
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			tr := telemetry.Track("auth.middleware")
			defer tr.Finish()

			// log each req with redacted headers
			logger.LogRequestFast(ctx)
			tr.Mark("log_request")

			// CORS preflight
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
			tr.Mark("cors")
			// - if method is not a standard method
			if string(ctx.Method()) == fasthttp.MethodOptions {
				ctx.SetStatusCode(fasthttp.StatusNoContent)
				return
			}

			// ip whitelist
			if len(cfg.IPWhitelist) > 0 {
				ip := clientIPFast(ctx)
				logger.Debug("ip_check", "ip", ip)
				if !ipWhitelisted(ip, cfg.IPWhitelist) {
					utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "forbidden")
					logger.Warn("request_blocked", "reason", "ip_not_whitelisted", "ip", ip, "path", string(ctx.Path()))
					return
				}
			}
			tr.Mark("ip_check")

			// extract possible api_key info
			role, key, hasAPIKey := authenticateFast(ctx, cfg)
			logger.Debug("auth_check", "role", role, "has_api_key", hasAPIKey)
			tr.Mark("authenticate")

			// allow access to health & ready checkeers
			if (string(ctx.Path()) == "/healthz" || string(ctx.Path()) == "/readyz") && string(ctx.Method()) == fasthttp.MethodGet {
				ctx.Request.Header.Set("X-Role-Name", "unauth")
				next(ctx)
				tr.Mark("health_check")
				return
			}

			// extract api_key <> role resolution
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
			tr.Mark("role_resolution")

			// enforce api_key required
			if role == RoleUnauth || !hasAPIKey {
				utils.JSONErrorFast(ctx, fasthttp.StatusUnauthorized, "unauthorized")
				logger.Warn("request_unauthorized", "path", string(ctx.Path()), "remote", ctx.RemoteAddr().String())
				tr.Mark("api_key_validation")
				return
			} else {
				ctx.Request.Header.Set("X-Role-Name", roleName)
			}
			tr.Mark("api_key_validation")

			// enforce frontend routes only
			if role == RoleFrontend && !frontendAllowedFast(ctx) {
				utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "forbidden")
				logger.Warn("request_forbidden", "reason", "frontend_not_allowed", "path", string(ctx.Path()))
				tr.Mark("frontend_check")
				return
			}
			tr.Mark("frontend_check")

			// enforce admin_key <> admin routes only
			if role == RoleAdmin {
				path := string(ctx.Path())
				if !strings.HasPrefix(path, "/admin") {
					utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "admin api keys may only access /admin routes")
					logger.Warn("admin_route_violation", "path", path, "remote", ctx.RemoteAddr().String())
					tr.Mark("admin_check")
					return
				}
			}
			tr.Mark("admin_check")

			// enforce rate_limiting per api key
			if !limiters.Allow(key) {
				utils.JSONErrorFast(ctx, fasthttp.StatusTooManyRequests, "rate limit exceeded")
				logger.Warn("rate_limited", "has_api_key", hasAPIKey, "path", string(ctx.Path()))
				tr.Mark("rate_limit")
				return
			}
			tr.Mark("rate_limit")

			// allow request through
			logger.Info("request_allowed", "method", string(ctx.Method()), "path", string(ctx.Path()), "role", ctx.Request.Header.Peek("X-Role-Name"))
			next(ctx)
			tr.Mark("allow_request")
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

func authenticateFast(ctx *fasthttp.RequestCtx, cfg SecConfig) (Role, string, bool) {
	auth := string(ctx.Request.Header.Peek("Authorization"))
	var key string

	// log raw Authorization and X-API-Key values
	logger.Info("auth_headers_received",
		"authorization_raw", auth,
		"x_api_key_raw", string(ctx.Request.Header.Peek("X-API-Key")),
		"remote", ctx.RemoteAddr().String(),
		"path", string(ctx.Path()),
	)

	// accept both "Bearer " and "bearer " prefixes, case-insensitive
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		key = strings.TrimSpace(auth[7:])
	}

	// fallback to x-api-key header if bearer is missing
	if key == "" {
		key = string(ctx.Request.Header.Peek("X-API-Key"))
	}

	// if still nothing, treat as unauthenticated (by ip)
	if key == "" {
		logger.Info("unauthenticated_request", "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()), "reason", "no_api_key")
		return RoleUnauth, clientIPFast(ctx), false
	}

	// admin keys take precedence
	if cfg.AdminKeys != nil {
		if _, ok := cfg.AdminKeys[key]; ok {
			logger.Info("api_key_authenticated", "role", "admin", "key_raw", key, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			return RoleAdmin, key, true
		}
	}
	// backend keys
	if cfg.BackendKeys != nil {
		if _, ok := cfg.BackendKeys[key]; ok {
			logger.Info("api_key_authenticated", "role", "backend", "key_raw", key, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			return RoleBackend, key, true
		}
	}
	// frontend keys
	if cfg.FrontendKeys != nil {
		if _, ok := cfg.FrontendKeys[key]; ok {
			logger.Info("api_key_authenticated", "role", "frontend", "key_raw", key, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
			return RoleFrontend, key, true
		}
	}
	// unrecognized api key, but present
	logger.Warn("unrecognized_api_key", "key_raw", key, "remote", ctx.RemoteAddr().String(), "path", string(ctx.Path()))
	return RoleUnauth, key, true
}

// check if the request is allowed for frontend
func frontendAllowedFast(ctx *fasthttp.RequestCtx) bool {
	path := string(ctx.Path())
	// allow get or post on /v1/messages
	if strings.HasPrefix(path, "/v1/messages") {
		return true
	}
	// allow any method on /v1/threads and sub-paths
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
