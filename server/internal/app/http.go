package app

import (
	"context"

	"github.com/valyala/fasthttp"
	router "progressdb/pkg/router"

	"progressdb/pkg/api"
	"progressdb/pkg/auth"
	"progressdb/pkg/banner"
	"progressdb/pkg/config"
	"progressdb/pkg/store"
)

// printBanner prints the startup banner and build info.
func (a *App) printBanner() {
	var srcs []string
	switch a.eff.Source {
	case "flags":
		srcs = append(srcs, "flags")
	case "env":
		srcs = append(srcs, "env")
	case "config":
		srcs = append(srcs, "config")
	}
	verStr := a.version
	if a.commit != "none" {
		verStr += " (" + a.commit + ")"
	}
	if a.buildDate != "unknown" {
		verStr += " @ " + a.buildDate
	}
	// Use the effective config to print richer startup info (encryption, source)
	banner.PrintWithEff(a.eff, verStr)
}

// readyzHandler handles the /readyz endpoint (fasthttp).
func (a *App) readyzHandlerFast(ctx *fasthttp.RequestCtx) {
	// check store
	if !store.Ready() {
		ctx.SetStatusCode(fasthttp.StatusServiceUnavailable)
		ctx.SetContentType("application/json")
		_, _ = ctx.WriteString("{\"status\":\"not ready\"}")
		return
	}
	if a.rc != nil {
		if err := a.rc.Health(); err != nil {
			ctx.SetStatusCode(fasthttp.StatusServiceUnavailable)
			_, _ = ctx.WriteString("{\"status\":\"kms unhealthy\"}")
			return
		}
	}
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetContentType("application/json")
	ver := a.version
	if ver == "" {
		ver = "dev"
	}
	_, _ = ctx.WriteString("{\"status\":\"ok\",\"version\":\"" + ver + "\"}")
}

// healthzHandler handles the /healthz endpoint (fasthttp).
func (a *App) healthzHandlerFast(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(fasthttp.StatusOK)
	_, _ = ctx.WriteString("{\"status\":\"ok\"}")
}

// startHTTP builds and starts the fasthttp server, returning a channel that delivers errors.
func (a *App) startHTTP(_ context.Context) <-chan error {
	// build security config for auth middleware
	secCfg := auth.SecConfig{
		AllowedOrigins: append([]string{}, a.eff.Config.Security.CORS.AllowedOrigins...),
		RPS:            a.eff.Config.Security.RateLimit.RPS,
		Burst:          a.eff.Config.Security.RateLimit.Burst,
		IPWhitelist:    append([]string{}, a.eff.Config.Security.IPWhitelist...),
		BackendKeys:    map[string]struct{}{},
		FrontendKeys:   map[string]struct{}{},
		AdminKeys:      map[string]struct{}{},
	}
	// fill backend keys
	for _, k := range a.eff.Config.Security.APIKeys.Backend {
		secCfg.BackendKeys[k] = struct{}{}
	}
	// fill frontend keys
	for _, k := range a.eff.Config.Security.APIKeys.Frontend {
		secCfg.FrontendKeys[k] = struct{}{}
	}
	// fill admin keys
	for _, k := range a.eff.Config.Security.APIKeys.Admin {
		secCfg.AdminKeys[k] = struct{}{}
	}

	// set runtime config for global use
	runtimeCfg := &config.RuntimeConfig{BackendKeys: map[string]struct{}{}, SigningKeys: map[string]struct{}{}}
	for _, k := range a.eff.Config.Security.APIKeys.Backend {
		runtimeCfg.BackendKeys[k] = struct{}{}
		runtimeCfg.SigningKeys[k] = struct{}{}
	}
	config.SetRuntime(runtimeCfg)

	// build fasthttp router and register API routes
	r := router.New()
	// health and ready handlers
	r.GET("/healthz", a.healthzHandlerFast)
	r.GET("/readyz", a.readyzHandlerFast)
	// Register API routes on fasthttp router
	// Signing endpoints
	r.POST("/_sign", api.Sign)
	r.POST("/v1/_sign", api.Sign)

	// Thread & message routes (centralized)
	r.POST("/v1/threads", api.CreateThread)
	r.GET("/v1/threads", api.ListThreads)
	r.PUT("/v1/threads/{id}", api.UpdateThread)
	r.GET("/v1/threads/{id}", api.GetThread)
	r.DELETE("/v1/threads/{id}", api.DeleteThread)

	r.POST("/v1/threads/{threadID}/messages", api.CreateThreadMessage)
	r.GET("/v1/threads/{threadID}/messages", api.ListThreadMessages)
	r.GET("/v1/threads/{threadID}/messages/{id}", api.GetThreadMessage)
	r.PUT("/v1/threads/{threadID}/messages/{id}", api.UpdateThreadMessage)
	r.DELETE("/v1/threads/{threadID}/messages/{id}", api.DeleteThreadMessage)

	r.GET("/v1/threads/{threadID}/messages/{id}/versions", api.ListMessageVersions)
	r.GET("/v1/threads/{threadID}/messages/{id}/reactions", api.GetReactions)
	r.POST("/v1/threads/{threadID}/messages/{id}/reactions", api.AddReaction)
	r.DELETE("/v1/threads/{threadID}/messages/{id}/reactions/{identity}", api.DeleteReaction)

	// Message-level routes
	r.POST("/v1/messages", api.CreateMessage)
	r.GET("/v1/messages", api.ListMessages)

	// Admin routes
	r.GET("/admin/health", api.AdminHealth)
	r.GET("/admin/stats", api.AdminStats)
	r.GET("/admin/threads", api.AdminListThreads)
	r.GET("/admin/keys", api.AdminListKeys)
	r.GET("/admin/keys/{key}", api.AdminGetKey)
	r.POST("/admin/encryption/rotate-thread-dek", api.AdminEncryptionRotateThreadDEK)
	r.POST("/admin/test-retention-run", api.AdminTestRetentionRun)
	r.NotFound(func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetContentType("application/json")
		_, _ = ctx.WriteString(`{"error":"not found"}`)
	})

	fastHandler := r.Handler
	// wrap with fasthttp native auth middleware
	fastHandler = auth.RequireSignedAuthorFast(fastHandler)
	fastHandler = auth.AuthenticateRequestMiddlewareFast(secCfg)(fastHandler)

	// create fasthttp server
	a.srvFast = &fasthttp.Server{Handler: fastHandler}

	// start server in goroutine and return error channel
	errCh := make(chan error, 1)
	go func() {
		// fasthttp has no direct TLS helper like ListenAndServeTLS here; for
		// simplicity run plain TCP. TLS can be handled by a proxy in production.
		errCh <- a.srvFast.ListenAndServe(a.eff.Addr)
	}()
	return errCh
}
