package app

import (
	"context"
	"net/http"

    "progressdb/pkg/api"
    "progressdb/pkg/auth"
    "progressdb/pkg/telemetry"
    "progressdb/pkg/banner"
    "progressdb/pkg/config"
    "progressdb/pkg/store"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger"
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

// setupHTTPHandlers sets up all HTTP handlers on the provided mux.
func (a *App) setupHTTPHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/readyz", a.readyzHandler)
	mux.Handle("/viewer/", http.StripPrefix("/viewer/", http.FileServer(http.Dir("./viewer"))))
	mux.HandleFunc("/healthz", healthzHandler)
	mux.Handle("/", api.Handler())
	mux.Handle("/docs/", httpSwagger.Handler(httpSwagger.URL("/openapi.yaml")))
	mux.Handle("/openapi.yaml", http.FileServer(http.Dir("./docs")))
	mux.Handle("/metrics", promhttp.Handler())
}

// readyzHandler handles the /readyz endpoint.
func (a *App) readyzHandler(w http.ResponseWriter, r *http.Request) {
	// check store
	if !store.Ready() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"status\":\"not ready\"}"))
		return
	}
	// check KMS if configured
	if a.rc != nil {
		if err := a.rc.Health(); err != nil {
			// RemoteClient may implement a more complete check
			if hc, ok := interface{}(a.rc).(interface{ HealthCheck() error }); ok {
				if derr := hc.HealthCheck(); derr != nil {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte("{\"status\":\"kms unhealthy\"}"))
					return
				}
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("{\"status\":\"kms unhealthy\"}"))
				return
			}
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	// include the running version to help ops verify what binary is active
	ver := a.version
	if ver == "" {
		ver = "dev"
	}
	_, _ = w.Write([]byte("{\"status\":\"ok\",\"version\":\"" + ver + "\"}"))
}

// healthzHandler handles the /healthz endpoint.
func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{\"status\":\"ok\"}"))
}

// startHTTP builds the handler, starts the HTTP server in a goroutine and
// returns a channel that will contain any server error.
func (a *App) startHTTP(_ context.Context) <-chan error {
	// create a new http mux
	mux := http.NewServeMux()
	a.setupHTTPHandlers(mux)

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

    // wrap mux with auth middleware, then telemetry middleware
    wrapped := auth.AuthenticateRequestMiddleware(secCfg)(mux)
    wrapped = telemetry.Middleware(wrapped)

	// create http server
	a.srv = &http.Server{Addr: a.eff.Addr, Handler: wrapped}

	// start server in goroutine and return error channel
	errCh := make(chan error, 1)
	go func() {
		cert := a.eff.Config.Server.TLS.CertFile
		key := a.eff.Config.Server.TLS.KeyFile
		if cert != "" && key != "" {
			errCh <- a.srv.ListenAndServeTLS(cert, key)
		} else {
			errCh <- a.srv.ListenAndServe()
		}
	}()
	return errCh
}
