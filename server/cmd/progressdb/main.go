package main

import (
    "flag"
    "log"
    "net/http"
    "os"
    "strings"

    "github.com/joho/godotenv"

    "progressdb/pkg/api"
    "progressdb/pkg/banner"
    "progressdb/pkg/config"
    "progressdb/pkg/security"
    "progressdb/pkg/store"
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

    if err := http.ListenAndServe(*addr, mux); err != nil {
        log.Fatal(err)
    }
}
