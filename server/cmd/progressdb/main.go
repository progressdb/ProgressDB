package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger"

	"progressdb/pkg/api"
	"progressdb/pkg/banner"
	"progressdb/pkg/config"
	"progressdb/pkg/kms"
	"progressdb/pkg/security"
	"progressdb/pkg/store"
	"progressdb/pkg/validation"
)

// generateHex removed; server no longer generates API keys (we rely on peer UIDs)

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
	// Embedded KEK is not used in external-only deployment; do not set master key here.
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
	// Always spawn the KMS child process and register the remote provider.
	socket := os.Getenv("PROGRESSDB_KMS_SOCKET")
	if socket == "" {
		socket = "/tmp/progressdb-kms.sock"
	}
	dataDir := os.Getenv("PROGRESSDB_KMS_DATA_DIR")
	if dataDir == "" {
		dataDir = "./kms-data"
	}

	var child *kms.CmdHandle
	// always spawn the child for production deployments
	bin := os.Getenv("PROGRESSDB_KMS_BINARY")
	if bin == "" {
		// KMS binary is in the same folder as this file (progressdb)
		exePath, err := os.Executable()
		if err != nil {
			log.Fatalf("failed to determine executable path: %v", err)
		}
		bin = filepath.Join(filepath.Dir(exePath), "kms")
	}
	args := []string{"--socket", socket, "--data-dir", dataDir}
	// pass allowed UIDs to child so it can accept peer-credential-authenticated
	// connections from this server process. We do NOT pass API keys to avoid
	// keeping secrets in the server process memory.
	env := map[string]string{
		"PROGRESSDB_KMS_ALLOWED_UIDS": fmt.Sprintf("%d", os.Getuid()),
	}

	// Determine encryption usage and require key file when enabled.
	useEnc := false
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PROGRESSDB_USE_ENCRYPTION"))) {
	case "1", "true", "yes":
		useEnc = true
	}
	if useEnc {
		// require master key file path
		mkFile := strings.TrimSpace(os.Getenv("PROGRESSDB_KMS_MASTER_KEY_FILE"))
		if mkFile == "" {
			log.Fatalf("PROGRESSDB_USE_ENCRYPTION=true but PROGRESSDB_KMS_MASTER_KEY_FILE is not set. Provide a path to a 64-hex (32-byte) key file.")
		}
		// pass file path to child
		env["PROGRESSDB_KMS_MASTER_KEY_FILE"] = mkFile
	}

	var rc *kms.RemoteClient // Declare rc here so it is available in the shutdown goroutine

	// Log encryption state for operators
	if useEnc {
		// rc is nil here, because it is only initialized after KMS child is started below.
		log.Printf("encryption enabled: true (KMS socket=%s)", socket)
	} else {
		log.Printf("encryption enabled: false")
	}
	var cancel context.CancelFunc
	if useEnc {
		ctx, cancelLocal := context.WithCancel(context.Background())
		cancel = cancelLocal
		// Create and bind the unix-domain socket in the parent so we can pass
		// the listener FD to the child. This avoids TOCTOU races on the
		// filesystem path. If we cannot create the listener for any reason we
		// fall back to letting the child bind the socket itself.
		var extraFiles []*os.File
		var parentListenerClose func()
		if socket != "" {
			// ensure socket directory exists
			if dir := filepath.Dir(socket); dir != "" {
				_ = os.MkdirAll(dir, 0700)
			}
			// try to create listener here
			if ln, err := net.Listen("unix", socket); err == nil {
				if ul, ok := ln.(*net.UnixListener); ok {
					if f, ferr := ul.File(); ferr == nil {
						extraFiles = append(extraFiles, f)
						// parent will close its copy of listener after spawn
						parentListenerClose = func() {
							_ = ln.Close()
							_ = f.Close()
						}
					} else {
						_ = ln.Close()
					}
				} else {
					_ = ln.Close()
				}
			}
		}

		ch, err := kms.StartChild(ctx, bin, args, socket, 0, 0, 10*time.Second, env, extraFiles)
		if parentListenerClose != nil {
			// close parent's copy of listener now that child inherited it
			parentListenerClose()
		}
		if err != nil {
			log.Fatalf("failed to start KMS: %v", err)
		}
		child = ch

		rc = kms.NewRemoteClient(socket)
		security.RegisterKMSProvider(rc)
		// Verify remote KMS is healthy before continuing; fail fast with
		// actionable message so operators know to install/start KMS.
		if err := rc.Health(); err != nil {
			log.Fatalf("KMS health check failed at %s: %v; ensure KMS is installed and PROGRESSDB_KMS_ALLOWED_UIDS permits this process", socket, err)
		}
	}
	// ensure child is stopped on shutdown
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigc
		log.Printf("signal received: %v, shutting down", s)
		// cancel background contexts (including child start/monitor)
		if cancel != nil {
			cancel()
		}
		if rc != nil {
			_ = rc.Close()
		}
		if child != nil {
			_ = child.Stop(5 * time.Second)
		}
		os.Exit(0)
	}()
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
	// API access always requires keys; no allow-unauth option.
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
	runtimeCfg := &config.RuntimeConfig{BackendKeys: map[string]struct{}{}, SigningKeys: map[string]struct{}{}}
	for k := range backendKeys {
		runtimeCfg.BackendKeys[k] = struct{}{}
		runtimeCfg.SigningKeys[k] = struct{}{}
	}
	for k := range signingKeys {
		runtimeCfg.SigningKeys[k] = struct{}{}
	}
	config.SetRuntime(runtimeCfg)

	wrapped := security.AuthenticateRequestMiddleware(secCfg)(mux)

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
