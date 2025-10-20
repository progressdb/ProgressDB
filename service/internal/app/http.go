package app

import (
	"context"
	"time"

	router "progressdb/pkg/router"

	"github.com/valyala/fasthttp"

	"net/http"
	"net/http/pprof"
	"progressdb/pkg/api"
	"progressdb/pkg/auth"
	"progressdb/pkg/banner"
	"progressdb/pkg/config"
	"progressdb/pkg/store"

	"github.com/valyala/fasthttp/fasthttpadaptor"
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
		AllowedOrigins: append([]string{}, a.eff.Config.Server.CORS.AllowedOrigins...),
		RPS:            a.eff.Config.Server.RateLimit.RPS,
		Burst:          a.eff.Config.Server.RateLimit.Burst,
		IPWhitelist:    append([]string{}, a.eff.Config.Server.IPWhitelist...),
		BackendKeys:    map[string]struct{}{},
		FrontendKeys:   map[string]struct{}{},
		AdminKeys:      map[string]struct{}{},
	}
	// fill backend keys
	for _, k := range a.eff.Config.Server.APIKeys.Backend {
		secCfg.BackendKeys[k] = struct{}{}
	}
	// fill frontend keys
	for _, k := range a.eff.Config.Server.APIKeys.Frontend {
		secCfg.FrontendKeys[k] = struct{}{}
	}
	// fill admin keys
	for _, k := range a.eff.Config.Server.APIKeys.Admin {
		secCfg.AdminKeys[k] = struct{}{}
	}

	// set runtime config for global use
	runtimeCfg := &config.RuntimeConfig{BackendKeys: map[string]struct{}{}, SigningKeys: map[string]struct{}{}}
	for _, k := range a.eff.Config.Server.APIKeys.Backend {
		runtimeCfg.BackendKeys[k] = struct{}{}
		runtimeCfg.SigningKeys[k] = struct{}{}
	}
	config.SetRuntime(runtimeCfg)

	// build fasthttp router and register API routes
	r := router.New()
	// health and ready handlers
	r.GET("/healthz", a.healthzHandlerFast)
	r.GET("/readyz", a.readyzHandlerFast)

	// pprof routes, protected by admin key
	wrapHTTPHandler := func(h http.Handler) func(ctx *fasthttp.RequestCtx) {
		return func(ctx *fasthttp.RequestCtx) {
			fasthttpadaptor.NewFastHTTPHandler(h)(ctx)
		}
	}
	r.GET("/admin/debug/pprof/", wrapHTTPHandler(http.HandlerFunc(pprof.Index)))
	r.GET("/admin/debug/pprof/cmdline", wrapHTTPHandler(http.HandlerFunc(pprof.Cmdline)))
	r.GET("/admin/debug/pprof/profile", wrapHTTPHandler(http.HandlerFunc(pprof.Profile)))
	r.GET("/admin/debug/pprof/symbol", wrapHTTPHandler(http.HandlerFunc(pprof.Symbol)))
	r.GET("/admin/debug/pprof/trace", wrapHTTPHandler(http.HandlerFunc(pprof.Trace)))

	// Register API routes on fasthttp router
	api.RegisterRoutes(r)

	r.NotFound(func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetContentType("application/json")
		_, _ = ctx.WriteString(`{"error":"not found"}`)
	})

	fastHandler := r.Handler
	// wrap with fasthttp native auth middleware
	fastHandler = auth.RequireSignedAuthorFast(fastHandler)
	fastHandler = auth.AuthenticateRequestMiddlewareFast(secCfg)(fastHandler)

	// create fasthttp.Server options for readability and maintainability
	const (
		readBufferSize       = 64 * 1024        // 64 KiB read buffer per connection
		maxRequestBodySize   = 5 * 1024 * 1024  // 5 MiB max request body
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
		ReduceMemoryUsage:    true, // reduces memory usage at the expense of performance
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
		errCh <- a.srvFast.ListenAndServe(a.eff.Addr)
	}()
	return errCh
}
