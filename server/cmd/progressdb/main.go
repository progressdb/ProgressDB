package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger"

	"progressdb/pkg/api"
	"progressdb/pkg/banner"
	"progressdb/pkg/config"
	"progressdb/pkg/security"
	"progressdb/pkg/store"
	"progressdb/pkg/validation"
)

func main() {
    // build metadata - set via ldflags during build/release
    var (
        version   = "dev"
        commit    = "none"
        buildDate = "unknown"
    )
	// Parse flags (moved into config package to centralize flag parsing)
	_ = godotenv.Load(".env")
	addrVal, dbVal, cfgVal, setFlags := config.ParseCommandFlags()

	// Resolve config path (file flag wins over env)
	cfgPath := config.ResolveConfigPath(cfgVal, setFlags["config"])

	// Load effective config (file + env) and canonical app-level config
	cfg, backendKeys, signingKeys, envUsed, err := config.LoadEffective(cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Apply loaded config values for encryption and validation, but respect
	// explicit flags: flags win over config/env when provided by the user.
	var addr string
	var dbPath string
	if !setFlags["addr"] {
		addr = cfg.Addr()
	} else {
		addr = addrVal
	}
	if !setFlags["db"] {
		if p := cfg.Storage.DBPath; p != "" {
			dbPath = p
		} else {
			dbPath = dbVal
		}
	} else {
		dbPath = dbVal
	}
	if cfg.Security.EncryptionKey != "" {
		if err := security.SetKeyHex(cfg.Security.EncryptionKey); err != nil {
			log.Fatalf("invalid encryption key: %v", err)
		}
	}
	if len(cfg.Security.Fields) > 0 {
		fields := make([]security.EncField, 0, len(cfg.Security.Fields))
		for _, f := range cfg.Security.Fields {
			fields = append(fields, security.EncField{Path: f.Path, Algorithm: f.Algorithm})
		}
		if err := security.SetFieldPolicy(fields); err != nil {
			log.Fatalf("invalid encryption fields: %v", err)
		}
	}
	// Validation rules
	vr := validation.Rules{Types: map[string]string{}, MaxLen: map[string]int{}, Enums: map[string][]string{}}
	vr.Required = append(vr.Required, cfg.Validation.Required...)
	for _, t := range cfg.Validation.Types {
		vr.Types[t.Path] = t.Type
	}
	for _, ml := range cfg.Validation.MaxLen {
		vr.MaxLen[ml.Path] = ml.Max
	}
	for _, e := range cfg.Validation.Enums {
		vr.Enums[e.Path] = append([]string{}, e.Values...)
	}
	for _, wt := range cfg.Validation.WhenThen {
		vr.WhenThen = append(vr.WhenThen, validation.WhenThenRule{
			WhenPath: wt.When.Path,
			Equals:   wt.When.Equals,
			ThenReq:  append([]string{}, wt.Then.Required...),
		})
	}
	validation.SetRules(vr)

	// Flags explicitly set win over env/config for addr and dbPath (handled above).

	if err := store.Open(dbPath); err != nil {
		log.Fatalf("failed to open pebble at %s: %v", dbPath, err)
	}
	// Determine config sources summary (flags/env/config)
	srcs := []string{}
	if len(setFlags) > 0 {
		srcs = append(srcs, "flags")
	}
	// detect env (based on LoadEffective result)
	if envUsed {
		srcs = append(srcs, "env")
	}
	// config file present?
	if _, err := config.Load(cfgPath); err == nil {
		srcs = append(srcs, "config")
	}
    // Include version/commit info in the startup banner when present.
    verStr := version
    if commit != "none" {
        verStr = verStr + " (" + commit + ")"
    }
    if buildDate != "unknown" {
        verStr = verStr + " @ " + buildDate
    }
    banner.Print(addr, dbPath, strings.Join(srcs, ", "), verStr)

	mux := http.NewServeMux()

    // Serve the web viewer at /viewer/
    mux.Handle("/viewer/", http.StripPrefix("/viewer/", http.FileServer(http.Dir("./viewer"))))

    // Liveness probe used by deployment systems and CI
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("{\"status\":\"ok\"}"))
    })

	// API handler (catch-all under /)
	mux.Handle("/", api.Handler())

	// Serve Swagger UI at /docs and the OpenAPI spec at /openapi.yaml
	mux.Handle("/docs/", httpSwagger.Handler(httpSwagger.URL("/openapi.yaml")))
	mux.Handle("/openapi.yaml", http.FileServer(http.Dir("./docs")))
	mux.Handle("/metrics", promhttp.Handler())

	// Build security middleware from config/env
	secCfg := security.SecConfig{
		AllowedOrigins: nil,
		RPS:            0,
		Burst:          0,
		IPWhitelist:    nil,
		BackendKeys:    map[string]struct{}{},
		FrontendKeys:   map[string]struct{}{},
		AdminKeys:      map[string]struct{}{},
		AllowUnauth:    false,
	}
	// Apply CORS, rate limits and security keys from effective cfg
	secCfg.AllowedOrigins = append(secCfg.AllowedOrigins, cfg.Security.CORS.AllowedOrigins...)
	if cfg.Security.RateLimit.RPS > 0 {
		secCfg.RPS = cfg.Security.RateLimit.RPS
	}
	if cfg.Security.RateLimit.Burst > 0 {
		secCfg.Burst = cfg.Security.RateLimit.Burst
	}
	if len(cfg.Security.IPWhitelist) > 0 {
		secCfg.IPWhitelist = append(secCfg.IPWhitelist, cfg.Security.IPWhitelist...)
	}
	secCfg.AllowUnauth = cfg.Security.APIKeys.AllowUnauth
	for k := range backendKeys {
		secCfg.BackendKeys[k] = struct{}{}
	}
	for _, k := range cfg.Security.APIKeys.Frontend {
		secCfg.FrontendKeys[k] = struct{}{}
	}
	for _, k := range cfg.Security.APIKeys.Admin {
		secCfg.AdminKeys[k] = struct{}{}
	}

	// Populate the global runtime config with backend and signing keys.
	rc := &config.RuntimeConfig{BackendKeys: map[string]struct{}{}, SigningKeys: map[string]struct{}{}}
	for k := range backendKeys {
		rc.BackendKeys[k] = struct{}{}
		rc.SigningKeys[k] = struct{}{}
	}
	for k := range signingKeys {
		rc.SigningKeys[k] = struct{}{}
	}
	config.SetRuntime(rc)

	wrapped := security.NewMiddleware(secCfg)(mux)

	// TLS support: use values from effective cfg
	cert := cfg.Server.TLS.CertFile
	key := cfg.Server.TLS.KeyFile
	var errServe error
	if cert != "" && key != "" {
		errServe = http.ListenAndServeTLS(addr, cert, key, wrapped)
	} else {
		errServe = http.ListenAndServe(addr, wrapped)
	}
	if errServe != nil {
		log.Fatal(errServe)
	}
}
