package auth

import (
	"net"
	"strings"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/logger"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/utils"
)

// role and secconfig types are defined in identity.go

// AuthenticateRequestMiddlewareFast is a fasthttp-native middleware equivalent
// to AuthenticateRequestMiddleware. It returns a middleware that wraps a
// fasthttp.RequestHandler.
func AuthenticateRequestMiddlewareFast(cfg SecConfig) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	limiters := &limiterPool{cfg: cfg}
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			// log request (redacts sensitive headers)
			logger.LogRequestFast(ctx)

			// CORS preflight
			origin := string(ctx.Request.Header.Peek("Origin"))
			if origin != "" && originAllowed(origin, cfg.AllowedOrigins) {
				ctx.Response.Header.Set("Access-Control-Allow-Origin", origin)
				ctx.Response.Header.Set("Vary", "Origin")
				ctx.Response.Header.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,PATCH,OPTIONS")
				ctx.Response.Header.Set("Access-Control-Max-Age", "600")
				ctx.Response.Header.Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-API-Key,X-User-ID,X-User-Signature")
				ctx.Response.Header.Set("Access-Control-Expose-Headers", "X-Role-Name")
			}
			if string(ctx.Method()) == fasthttp.MethodOptions {
				ctx.SetStatusCode(fasthttp.StatusNoContent)
				return
			}

			// IP whitelist
			if len(cfg.IPWhitelist) > 0 {
				ip := clientIPFast(ctx)
				logger.Debug("ip_check", "ip", ip)
				if !ipWhitelisted(ip, cfg.IPWhitelist) {
					utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "forbidden")
					logger.Warn("request_blocked", "reason", "ip_not_whitelisted", "ip", ip, "path", string(ctx.Path()))
					return
				}
			}

			// auth
			authSpan := telemetry.StartSpanNoCtx("auth.authenticate")
			role, key, hasAPIKey := authenticateFast(ctx, cfg)
			authSpan()
			logger.Debug("auth_check", "role", role, "has_api_key", hasAPIKey)

			// allow unauthenticated health checks for probes
			if (string(ctx.Path()) == "/healthz" || string(ctx.Path()) == "/readyz") && string(ctx.Method()) == fasthttp.MethodGet {
				ctx.Request.Header.Set("X-Role-Name", "unauth")
				next(ctx)
				return
			}

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

			if role == RoleUnauth || !hasAPIKey {
				utils.JSONErrorFast(ctx, fasthttp.StatusUnauthorized, "unauthorized")
				logger.Warn("request_unauthorized", "path", string(ctx.Path()), "remote", ctx.RemoteAddr().String())
				return
			} else {
				ctx.Request.Header.Set("X-Role-Name", roleName)
			}

			if role == RoleFrontend && !frontendAllowedFast(ctx) {
				utils.JSONErrorFast(ctx, fasthttp.StatusForbidden, "forbidden")
				logger.Warn("request_forbidden", "reason", "frontend_not_allowed", "path", string(ctx.Path()))
				return
			}

			rlSpan := telemetry.StartSpanNoCtx("auth.rate_limit")
			if !limiters.Allow(key) {
				rlSpan()
				utils.JSONErrorFast(ctx, fasthttp.StatusTooManyRequests, "rate limit exceeded")
				logger.Warn("rate_limited", "has_api_key", hasAPIKey, "path", string(ctx.Path()))
				return
			}
			rlSpan()

			logger.Info("request_allowed", "method", string(ctx.Method()), "path", string(ctx.Path()), "role", ctx.Request.Header.Peek("X-Role-Name"))

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

func authenticateFast(ctx *fasthttp.RequestCtx, cfg SecConfig) (Role, string, bool) {
	auth := string(ctx.Request.Header.Peek("Authorization"))
	var key string
	if len(auth) > 7 && strings.ToLower(auth[:7]) == "bearer " {
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
	if _, ok := cfg.BackendKeys[key]; ok {
		return RoleBackend, key, true
	}
	if _, ok := cfg.FrontendKeys[key]; ok {
		return RoleFrontend, key, true
	}
	return RoleUnauth, key, true
}

func frontendAllowedFast(ctx *fasthttp.RequestCtx) bool {
	path := string(ctx.Path())
	method := string(ctx.Method())
	if path == "/v1/messages" && (method == fasthttp.MethodGet || method == fasthttp.MethodPost) {
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
