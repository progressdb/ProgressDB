package app

import (
	"context"
	"net"
	"strings"
	"time"

	router "progressdb/pkg/api/router"
	storedb "progressdb/pkg/store/db/storedb"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/api"
	"progressdb/pkg/api/auth"
	"progressdb/pkg/config"
	"progressdb/pkg/config/banner"
)

func (a *App) printBanner() {
	verStr := a.version
	if a.commit != "none" {
		verStr += " (" + a.commit + ")"
	}
	if a.buildDate != "unknown" {
		verStr += " @ " + a.buildDate
	}
	// Use the global config to print richer startup info
	cfg := config.GetConfig()
	eff := config.EffectiveConfigResult{
		Config: cfg,
		Addr:   cfg.Addr(),
		DBPath: cfg.Server.DBPath,
		Source: "config",
	}
	banner.PrintWithEff(eff, verStr)
}

func (a *App) readyzHandlerFast(ctx *fasthttp.RequestCtx) {
	// check store
	if !storedb.Ready() {
		router.WriteJSONError(ctx, fasthttp.StatusServiceUnavailable, "not ready")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ver := a.version
	if ver == "" {
		ver = "dev"
	}
	router.WriteJSONOk(ctx, map[string]interface{}{
		"status":  "ok",
		"version": ver,
	})
}
func (a *App) healthzHandlerFast(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/json")
	router.WriteJSONOk(ctx, map[string]interface{}{
		"status": "ok",
	})
}

// starts the http server and returns cancel functionality
func (a *App) startHTTP(_ context.Context) <-chan error {
	cfg := config.GetConfig()
	// build security config for auth middleware
	secCfg := auth.SecConfig{
		AllowedOrigins: append([]string{}, cfg.Server.CORS.AllowedOrigins...),
		RPS:            cfg.Server.RateLimit.RPS,
		Burst:          cfg.Server.RateLimit.Burst,
		IPWhitelist:    append([]string{}, cfg.Server.IPWhitelist...),
		BackendKeys:    map[string]struct{}{},
		FrontendKeys:   map[string]struct{}{},
		AdminKeys:      map[string]struct{}{},
	}
	// fill backend keys
	for _, k := range cfg.Server.APIKeys.Backend {
		secCfg.BackendKeys[k] = struct{}{}
	}
	// fill frontend keys
	for _, k := range cfg.Server.APIKeys.Frontend {
		secCfg.FrontendKeys[k] = struct{}{}
	}
	// fill admin keys
	for _, k := range cfg.Server.APIKeys.Admin {
		secCfg.AdminKeys[k] = struct{}{}
	}

	// http router registration
	r := router.New()
	r.GET("/healthz", a.healthzHandlerFast)
	r.GET("/readyz", a.readyzHandlerFast)
	api.RegisterRoutes(r)

	r.NotFound(func(ctx *fasthttp.RequestCtx) {
		router.WriteJSONError(ctx, fasthttp.StatusNotFound, "not found")
	})

	fastHandler := r.Handler
	// wrap with fasthttp native auth middleware
	fastHandler = auth.AuthenticateRequestMiddleware(secCfg)(fastHandler)

	// create fasthttp.Server options for readability and maintainability
	maxRequestBodySize := config.GetMaxPayloadSize()

	const (
		readBufferSize       = 64 * 1024        // 64 KiB read buffer per connection
		concurrency          = 0                // unlimited concurrency (0 means unlimited in fasthttp)
		readTimeout          = 10 * time.Second // timeout for reading request
		writeTimeout         = 10 * time.Second // timeout for writing response
		idleTimeout          = 30 * time.Second // max keep-alive idle duration per connection
		maxKeepaliveDuration = 2 * time.Minute  // max duration for keep-alive connection
	)

	a.srvFast = &fasthttp.Server{
		Handler:              fastHandler,
		ReadBufferSize:       readBufferSize,
		MaxRequestBodySize:   maxRequestBodySize,
		Concurrency:          concurrency,
		ReduceMemoryUsage:    false, // reduces memory usage at the expense of performance
		ReadTimeout:          readTimeout,
		WriteTimeout:         writeTimeout,
		IdleTimeout:          idleTimeout,
		MaxKeepaliveDuration: maxKeepaliveDuration,
	}

	// start server in goroutine and return error channel
	errCh := make(chan error, 1)
	go func() {
		// fasthttp has no direct TLS helper like ListenAndServeTLS here; for
		// simplicity run plain TCP. TLS can be handled by a proxy in production.
		cfg := config.GetConfig()
		addr := cfg.Addr()

		// Check if user explicitly configured IPv6 address
		// Default to IPv4 for maximum performance, opt-in to IPv6 with explicit config
		if cfg.Server.Address == "::" || (strings.Contains(cfg.Server.Address, ":") && cfg.Server.Address != "") {
			// User explicitly wants IPv6 - use custom listener (slight performance trade-off)
			listener, err := net.Listen("tcp", addr)
			if err != nil {
				errCh <- err
				return
			}
			errCh <- a.srvFast.Serve(listener)
		} else {
			// Default IPv4 path - maximum performance with fasthttp.ListenAndServe
			errCh <- a.srvFast.ListenAndServe(addr)
		}
	}()
	return errCh
}
