package main

import (
    "flag"
    "log"
    "net/http"
    "os"
    "strings"
    "strconv"

    "github.com/joho/godotenv"
    httpSwagger "github.com/swaggo/http-swagger"
    "github.com/prometheus/client_golang/prometheus/promhttp"

    "progressdb/pkg/api"
    "progressdb/pkg/banner"
    "progressdb/pkg/config"
    "progressdb/pkg/security"
    secmw "progressdb/pkg/security"
    "progressdb/pkg/store"
    "progressdb/pkg/validation"
)

func main() {
    addr := flag.String("addr", ":8080", "HTTP listen address")
    dbPath := flag.String("db", "./.database", "Pebble DB path")
    cfgPath := flag.String("config", "./config.yaml", "Path to config file")
    flag.Parse()

    // Track which flags were explicitly provided to enforce precedence.
    setFlags := map[string]bool{}
    flag.CommandLine.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })

    // Load environment from .env if present.
    _ = godotenv.Load(".env")

    // Allow env to specify a config path if flag not set.
    if !setFlags["config"] {
        if p := os.Getenv("PROGRESSDB_CONFIG"); p != "" {
            *cfgPath = p
        }
    }

    // Load config if available; override defaults.
    if cfg, err := config.Load(*cfgPath); err == nil {
        if a := cfg.Addr(); a != "" {
            *addr = a
        }
        if p := cfg.Storage.DBPath; p != "" {
            *dbPath = p
        }
        if err := security.SetKeyHex(cfg.Security.EncryptionKey); err != nil {
            log.Fatalf("invalid encryption key: %v", err)
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
    }

    // Environment overrides
    if v := os.Getenv("PROGRESSDB_ADDR"); v != "" {
        *addr = v
    } else {
        // Compose from ADDRESS + PORT if provided separately
        host := os.Getenv("PROGRESSDB_ADDRESS")
        port := os.Getenv("PROGRESSDB_PORT")
        if host != "" || port != "" {
            // Default host and port fallbacks
            if host == "" {
                host = "0.0.0.0"
            }
            if port == "" {
                port = "8080"
            }
            *addr = host + ":" + port
        }
    }
    if v := os.Getenv("PROGRESSDB_DB_PATH"); v != "" {
        *dbPath = v
    }
    if v := os.Getenv("PROGRESSDB_ENCRYPTION_KEY"); v != "" {
        if err := security.SetKeyHex(v); err != nil {
            log.Fatalf("invalid encryption key from env: %v", err)
        }
    }
    if v := os.Getenv("PROGRESSDB_ENCRYPT_FIELDS"); v != "" {
        // Comma-separated paths; algorithm defaults to aes-gcm
        parts := strings.Split(v, ",")
        fields := make([]security.EncField, 0, len(parts))
        for _, p := range parts {
            p = strings.TrimSpace(p)
            if p == "" {
                continue
            }
            fields = append(fields, security.EncField{Path: p, Algorithm: "aes-gcm"})
        }
        if err := security.SetFieldPolicy(fields); err != nil {
            log.Fatalf("invalid PROGRESSDB_ENCRYPT_FIELDS: %v", err)
        }
    }

    // Flags explicitly set win over env/config for addr and dbPath.
    if setFlags["addr"] {
        // leave *addr as-is
    }
    if setFlags["db"] {
        // leave *dbPath as-is
    }

    if err := store.Open(*dbPath); err != nil {
        log.Fatalf("failed to open pebble at %s: %v", *dbPath, err)
    }

	banner.Print(*addr, *dbPath)

    mux := http.NewServeMux()
    mux.Handle("/", api.Handler())

    // Serve Swagger UI at /docs and the OpenAPI spec at /openapi.yaml
    mux.Handle("/docs/", httpSwagger.Handler(httpSwagger.URL("/openapi.yaml")))
    mux.Handle("/openapi.yaml", http.FileServer(http.Dir("./docs")))
    mux.Handle("/metrics", promhttp.Handler())

    // Build security middleware from config/env
    secCfg := secmw.SecConfig{
        AllowedOrigins: nil,
        RPS:            0,
        Burst:          0,
        IPWhitelist:    nil,
        BackendKeys:    map[string]struct{}{},
        FrontendKeys:   map[string]struct{}{},
        AdminKeys:      map[string]struct{}{},
        AllowUnauth:    false,
    }
    // CORS allowed origins from env (comma-separated) or config
    if v := os.Getenv("PROGRESSDB_CORS_ORIGINS"); v != "" {
        for _, s := range strings.Split(v, ",") {
            s = strings.TrimSpace(s)
            if s != "" {
                secCfg.AllowedOrigins = append(secCfg.AllowedOrigins, s)
            }
        }
    } else if cfg, err := config.Load(*cfgPath); err == nil {
        secCfg.AllowedOrigins = append(secCfg.AllowedOrigins, cfg.Security.CORS.AllowedOrigins...)
    }
    // Rate limit from env or config
    if v := os.Getenv("PROGRESSDB_RATE_RPS"); v != "" {
        if f, err := parseFloat(v); err == nil { secCfg.RPS = f }
    }
    if v := os.Getenv("PROGRESSDB_RATE_BURST"); v != "" {
        if n, err := parseInt(v); err == nil { secCfg.Burst = n }
    }
    if cfg, err := config.Load(*cfgPath); err == nil {
        if secCfg.RPS == 0 && cfg.Security.RateLimit.RPS > 0 { secCfg.RPS = cfg.Security.RateLimit.RPS }
        if secCfg.Burst == 0 && cfg.Security.RateLimit.Burst > 0 { secCfg.Burst = cfg.Security.RateLimit.Burst }
        if len(secCfg.IPWhitelist) == 0 && len(cfg.Security.IPWhitelist) > 0 { secCfg.IPWhitelist = append(secCfg.IPWhitelist, cfg.Security.IPWhitelist...) }
        secCfg.AllowUnauth = cfg.Security.APIKeys.AllowUnauth || secCfg.AllowUnauth
        for _, k := range cfg.Security.APIKeys.Backend { secCfg.BackendKeys[k] = struct{}{} }
        for _, k := range cfg.Security.APIKeys.Frontend { secCfg.FrontendKeys[k] = struct{}{} }
        for _, k := range cfg.Security.APIKeys.Admin { secCfg.AdminKeys[k] = struct{}{} }
    }
    if v := os.Getenv("PROGRESSDB_API_BACKEND_KEYS"); v != "" {
        for _, k := range strings.Split(v, ",") { k = strings.TrimSpace(k); if k != "" { secCfg.BackendKeys[k] = struct{}{} } }
    }
    if v := os.Getenv("PROGRESSDB_API_FRONTEND_KEYS"); v != "" {
        for _, k := range strings.Split(v, ",") { k = strings.TrimSpace(k); if k != "" { secCfg.FrontendKeys[k] = struct{}{} } }
    }
    if v := os.Getenv("PROGRESSDB_API_ADMIN_KEYS"); v != "" {
        for _, k := range strings.Split(v, ",") { k = strings.TrimSpace(k); if k != "" { secCfg.AdminKeys[k] = struct{}{} } }
    }
    if v := os.Getenv("PROGRESSDB_ALLOW_UNAUTH"); v != "" {
        secCfg.AllowUnauth = strings.ToLower(v) == "true" || v == "1"
    }

    wrapped := secmw.NewMiddleware(secCfg)(mux)

    // TLS support
    cert := os.Getenv("PROGRESSDB_TLS_CERT")
    key := os.Getenv("PROGRESSDB_TLS_KEY")
    if cert == "" || key == "" {
        if cfg, err := config.Load(*cfgPath); err == nil {
            if cfg.Server.TLS.CertFile != "" { cert = cfg.Server.TLS.CertFile }
            if cfg.Server.TLS.KeyFile  != "" { key  = cfg.Server.TLS.KeyFile }
        }
    }
    var errServe error
    if cert != "" && key != "" {
        errServe = http.ListenAndServeTLS(*addr, cert, key, wrapped)
    } else {
        errServe = http.ListenAndServe(*addr, wrapped)
    }
    if errServe != nil {
        log.Fatal(errServe)
    }
}

func parseFloat(s string) (float64, error) { return strconv.ParseFloat(strings.TrimSpace(s), 64) }
func parseInt(s string) (int, error) { v, err := strconv.Atoi(strings.TrimSpace(s)); return v, err }
