package auth

import (
	"net"
	"strings"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api/router"
	"progressdb/pkg/api/utils"
	"progressdb/pkg/state/logger"
)

func AuthenticateRequestMiddleware(cfg SecConfig) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	limiters := &limiterPool{cfg: cfg}
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			logger.LogRequestFast(ctx)

			// cors headers and handle options shortcut
			origin := utils.GetHeader(ctx, "Origin")
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
					logger.Warn("request_blocked", "reason", "ip_not_whitelisted", "ip", ip, "path", utils.GetPath(ctx))
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
				logger.Warn("request_unauthorized", "path", utils.GetPath(ctx), "remote", ctx.RemoteAddr().String())
				router.WriteJSONError(ctx, fasthttp.StatusUnauthorized, "unauthorized")
				return
			}

			// set for downstream usage
			ctx.Request.Header.Set("X-Role-Name", roleName)

			// request info for logging
			path := utils.GetPath(ctx)
			remote := ctx.RemoteAddr().String()
			reqInfo := []interface{}{"path", path, "remote", remote}

			// explicit role-path handling with specific logic per combination
			switch role {
			case RoleAdmin:
				if !utils.HasPathPrefix(ctx, "/admin") {
					router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
					logger.Warn("admin_path_denied", reqInfo...)
					return
				}
				logger.Debug("admin_path_allowed", reqInfo...)
			case RoleBackend:
				if !utils.HasPathPrefix(ctx, "/backend") && !utils.HasPathPrefix(ctx, "/frontend") {
					router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
					logger.Warn("backend_path_denied", reqInfo...)
					return
				}
				logger.Debug("backend_path_allowed", reqInfo...)
			case RoleFrontend:
				if !utils.HasPathPrefix(ctx, "/frontend") {
					router.WriteJSONError(ctx, fasthttp.StatusForbidden, "forbidden")
					logger.Warn("frontend_path_denied", reqInfo...)
					return
				}
				logger.Debug("frontend_path_allowed", reqInfo...)

				// frontend requires signature verification
				if !utils.HasUserSignature(ctx) {
					router.WriteJSONError(ctx, fasthttp.StatusUnauthorized, "signature required")
					logger.Warn("frontend_missing_signature", reqInfo...)
					return
				}
				RequireSignedAuthorMiddleware(next)(ctx)
				return
			}

			// rate limiting (per-key)
			if !limiters.Allow(key) {
				router.WriteJSONError(ctx, fasthttp.StatusTooManyRequests, "rate limit exceeded")
				logger.Warn("rate_limited", "has_api_key", hasAPIKey, "path", path)
				return
			}

			// authorized: continue to handler for admin/backend
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
	key := utils.ExtractAPIKey(ctx)

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
	path := utils.GetPath(ctx)
	method := string(ctx.Method())

	if (path == "/healthz" || path == "/readyz") && method == fasthttp.MethodGet {
		return true
	}

	return false
}
