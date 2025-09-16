package main

import (
	"context"
	"encoding/hex"
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

func main() {
	// Build metadata (set via ldflags at build/release)
	var (
		version   = "dev"
		commit    = "none"
		buildDate = "unknown"
	)

	_ = godotenv.Load(".env") // Load .env if present (no error if missing)

	// load config options
	flags := config.ParseConfigFlags()
	fileCfg, fileExists, err := config.ParseConfigFile(flags)
	if err != nil {
		log.Fatalf("failed to load config file: %v", err)
	}
	envCfg, envRes := config.ParseConfigEnvs()

	// load effective config (chooses a single source according to policy)
	eff, err := config.LoadEffectiveConfig(flags, fileCfg, fileExists, envCfg, envRes)
	if err != nil {
		log.Fatalf("failed to build effective config: %v", err)
	}

	// publish runtime keys based on chosen effective config (no mixing)
	bk := map[string]struct{}{}
	for _, k := range eff.Config.Security.APIKeys.Backend {
		bk[k] = struct{}{}
	}
	sk := map[string]struct{}{}
	for _, k := range eff.Config.Security.APIKeys.Backend {
		sk[k] = struct{}{}
	}
	config.SetRuntime(&config.RuntimeConfig{BackendKeys: bk, SigningKeys: sk})

	// --- Encryption field policy setup ---
	var fieldSrc []config.FieldEntry
	switch {
	case len(eff.Config.Security.Encryption.Fields) > 0:
		fieldSrc = eff.Config.Security.Encryption.Fields
	case len(eff.Config.Security.Fields) > 0:
		fieldSrc = eff.Config.Security.Fields
	}
	if len(fieldSrc) > 0 {
		fields := make([]security.EncField, 0, len(fieldSrc))
		for _, f := range fieldSrc {
			fields = append(fields, security.EncField{Path: f.Path, Algorithm: f.Algorithm})
		}
		if err := security.SetFieldPolicy(fields); err != nil {
			log.Fatalf("invalid encryption fields: %v", err)
		}
	}

	// --- Validation rules setup ---
	vr := validation.Rules{
		Types:  map[string]string{},
		MaxLen: map[string]int{},
		Enums:  map[string][]string{},
	}
	vr.Required = append(vr.Required, eff.Config.Validation.Required...)
	for _, t := range eff.Config.Validation.Types {
		vr.Types[t.Path] = t.Type
	}
	for _, ml := range eff.Config.Validation.MaxLen {
		vr.MaxLen[ml.Path] = ml.Max
	}
	for _, e := range eff.Config.Validation.Enums {
		vr.Enums[e.Path] = append([]string{}, e.Values...)
	}
	for _, wt := range eff.Config.Validation.WhenThen {
		vr.WhenThen = append(vr.WhenThen, validation.WhenThenRule{
			WhenPath: wt.When.Path,
			Equals:   wt.When.Equals,
			ThenReq:  append([]string{}, wt.Then.Required...),
		})
	}
	validation.SetRules(vr)

	// --- Open DB ---
	if err := store.Open(eff.DBPath); err != nil {
		log.Fatalf("failed to open pebble at %s: %v", eff.DBPath, err)
	}

	// --- KMS/Encryption setup ---
	socket := os.Getenv("PROGRESSDB_KMS_SOCKET")
	if socket == "" {
		socket = "/tmp/progressdb-kms.sock"
	}
	dataDir := os.Getenv("PROGRESSDB_KMS_DATA_DIR")
	if dataDir == "" {
		dataDir = "./kms-data"
	}
	bin := os.Getenv("PROGRESSDB_KMS_BINARY")
	if bin == "" {
		exePath, err := os.Executable()
		if err != nil {
			log.Fatalf("failed to determine executable path: %v", err)
		}
		bin = filepath.Join(filepath.Dir(exePath), "kms")
	}

	var (
		kmsCfgPath string
		child      *kms.CmdHandle
		rc         *kms.RemoteClient
		cancel     context.CancelFunc
	)

	// Determine if encryption is enabled (env overrides config)
	useEnc := eff.Config.Security.Encryption.Use
	if ev := strings.TrimSpace(os.Getenv("PROGRESSDB_USE_ENCRYPTION")); ev != "" {
		switch strings.ToLower(ev) {
		case "1", "true", "yes":
			useEnc = true
		default:
			useEnc = false
		}
	}

	if useEnc {
		// --- Master key selection and validation ---
		var mk string
		switch {
		case strings.TrimSpace(eff.Config.Security.KMS.MasterKeyFile) != "":
			mkFile := strings.TrimSpace(eff.Config.Security.KMS.MasterKeyFile)
			keyb, err := os.ReadFile(mkFile)
			if err != nil {
				log.Fatalf("failed to read master key file %s: %v", mkFile, err)
			}
			mk = strings.TrimSpace(string(keyb))
		case strings.TrimSpace(eff.Config.Security.KMS.MasterKeyHex) != "":
			mk = strings.TrimSpace(eff.Config.Security.KMS.MasterKeyHex)
		default:
			log.Fatalf("PROGRESSDB_USE_ENCRYPTION=true but no master key provided in server config. Set security.kms.master_key_file or security.kms.master_key_hex")
		}
		if mk == "" {
			log.Fatalf("master key is empty")
		}
		if kb, err := hex.DecodeString(mk); err != nil || len(kb) != 32 {
			log.Fatalf("invalid master_key_hex: must be 64-hex (32 bytes)")
		}

		// --- Write secure KMS launcher config ---
		lcfg := &kms.LauncherConfig{MasterKeyHex: mk, Socket: socket, DataDir: dataDir}
		kmsCfgPath, err = kms.CreateSecureConfigFile(lcfg, dataDir)
		if err != nil {
			log.Fatalf("failed to write kms config: %v", err)
		}
	}

	// --- Log encryption state ---
	if useEnc {
		log.Printf("encryption enabled: true (KMS socket=%s)", socket)
	} else {
		log.Printf("encryption enabled: false")
	}

	// --- KMS child process and remote client ---
	if useEnc {
		ctx, cancelLocal := context.WithCancel(context.Background())
		cancel = cancelLocal

		// Try to pre-bind the unix socket in parent (avoid TOCTOU)
		var (
			parentListenerClose func()
			ln                  *net.UnixListener
		)
		if socket != "" {
			if dir := filepath.Dir(socket); dir != "" {
				_ = os.MkdirAll(dir, 0700)
			}
			if l, err := net.Listen("unix", socket); err == nil {
				if ul, ok := l.(*net.UnixListener); ok {
					ln = ul
					if f, ferr := ul.File(); ferr == nil {
						parentListenerClose = func() {
							_ = ul.Close()
							_ = f.Close()
						}
					} else {
						_ = ul.Close()
					}
				} else {
					_ = l.Close()
				}
			}
		}

		// Start KMS child process (launcher reads config file)
		h, err := kms.StartChildLauncher(ctx, bin, kmsCfgPath, ln)
		if parentListenerClose != nil {
			parentListenerClose()
		}
		if err != nil {
			log.Fatalf("failed to start KMS: %v", err)
		}
		child = &kms.CmdHandle{Cmd: h.Cmd}
		rc = kms.NewRemoteClient(socket)
		security.RegisterKMSProvider(rc)
		if err := rc.Health(); err != nil {
			log.Fatalf("KMS health check failed at %s: %v; ensure KMS is installed and reachable", socket, err)
		}
	}

	// --- Graceful shutdown: stop KMS, close remote client, cancel contexts ---
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigc
		log.Printf("signal received: %v, shutting down", s)
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

	// --- Banner: show config sources and build info ---
	var srcs []string
	switch eff.Source {
	case "flags":
		srcs = append(srcs, "flags")
	case "env":
		srcs = append(srcs, "env")
	case "config":
		srcs = append(srcs, "config")
	}
	verStr := version
	if commit != "none" {
		verStr += " (" + commit + ")"
	}
	if buildDate != "unknown" {
		verStr += " @ " + buildDate
	}
	banner.Print(eff.Addr, eff.DBPath, strings.Join(srcs, ", "), verStr)

	// --- HTTP server setup ---
	mux := http.NewServeMux()
	mux.Handle("/viewer/", http.StripPrefix("/viewer/", http.FileServer(http.Dir("./viewer")))) // Web viewer
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {                   // Liveness probe
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ok\"}"))
	})
	mux.Handle("/", api.Handler())                                              // API handler
	mux.Handle("/docs/", httpSwagger.Handler(httpSwagger.URL("/openapi.yaml"))) // Swagger UI
	mux.Handle("/openapi.yaml", http.FileServer(http.Dir("./docs")))            // OpenAPI spec
	mux.Handle("/metrics", promhttp.Handler())                                  // Prometheus metrics

	// --- Security middleware config ---
	secCfg := security.SecConfig{
		AllowedOrigins: append([]string{}, eff.Config.Security.CORS.AllowedOrigins...),
		RPS:            eff.Config.Security.RateLimit.RPS,
		Burst:          eff.Config.Security.RateLimit.Burst,
		IPWhitelist:    append([]string{}, eff.Config.Security.IPWhitelist...),
		BackendKeys:    map[string]struct{}{},
		FrontendKeys:   map[string]struct{}{},
		AdminKeys:      map[string]struct{}{},
	}
	for _, k := range eff.Config.Security.APIKeys.Backend {
		secCfg.BackendKeys[k] = struct{}{}
	}
	for _, k := range eff.Config.Security.APIKeys.Frontend {
		secCfg.FrontendKeys[k] = struct{}{}
	}
	for _, k := range eff.Config.Security.APIKeys.Admin {
		secCfg.AdminKeys[k] = struct{}{}
	}

	// --- Global runtime config: backend/signing keys ---
	runtimeCfg := &config.RuntimeConfig{
		BackendKeys: map[string]struct{}{},
		SigningKeys: map[string]struct{}{},
	}
	for _, k := range eff.Config.Security.APIKeys.Backend {
		runtimeCfg.BackendKeys[k] = struct{}{}
		runtimeCfg.SigningKeys[k] = struct{}{}
	}
	for _, k := range eff.Config.Security.APIKeys.Backend {
		runtimeCfg.SigningKeys[k] = struct{}{}
	}
	config.SetRuntime(runtimeCfg)

	// --- Wrap mux with security/auth middleware ---
	wrapped := security.AuthenticateRequestMiddleware(secCfg)(mux)

	// --- Serve HTTP (TLS if configured) ---
	cert := eff.Config.Server.TLS.CertFile
	key := eff.Config.Server.TLS.KeyFile
	var errServe error
	if cert != "" && key != "" {
		errServe = http.ListenAndServeTLS(eff.Addr, cert, key, wrapped)
	} else {
		errServe = http.ListenAndServe(eff.Addr, wrapped)
	}
	if errServe != nil {
		log.Fatal(errServe)
	}
}
