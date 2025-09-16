package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"progressdb/pkg/api"
	"progressdb/pkg/banner"
	"progressdb/pkg/config"
	"progressdb/pkg/kms"
	"progressdb/pkg/security"
	"progressdb/pkg/store"
	"progressdb/pkg/validation"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	httpSwagger "github.com/swaggo/http-swagger"
)

// App encapsulates the server components and lifecycle.
type App struct {
	eff       config.EffectiveConfigResult
	version   string
	commit    string
	buildDate string

	// KMS/runtime
	child  *kms.CmdHandle
	rc     *kms.RemoteClient
	cancel context.CancelFunc

	srv *http.Server
}

// New initializes resources that do not require a running context (DB,
// validation, field policy, runtime keys). It does not start KMS or the
// HTTP server; call Run to start those and block until shutdown.
func New(eff config.EffectiveConfigResult, version, commit, buildDate string) (*App, error) {
	_ = godotenv.Load(".env")

	// runtime keys
	runtimeCfg := &config.RuntimeConfig{BackendKeys: map[string]struct{}{}, SigningKeys: map[string]struct{}{}}
	for _, k := range eff.Config.Security.APIKeys.Backend {
		runtimeCfg.BackendKeys[k] = struct{}{}
		runtimeCfg.SigningKeys[k] = struct{}{}
	}
	config.SetRuntime(runtimeCfg)

	// field policy
	if err := initFieldPolicy(eff); err != nil {
		return nil, fmt.Errorf("invalid encryption fields: %w", err)
	}

	// validation rules
	initValidation(eff)

	// open store
	if err := store.Open(eff.DBPath); err != nil {
		return nil, fmt.Errorf("failed to open pebble at %s: %w", eff.DBPath, err)
	}

	a := &App{eff: eff, version: version, commit: commit, buildDate: buildDate}
	return a, nil
}

// Run starts KMS (if enabled) and the HTTP server, and blocks until ctx is
// canceled or a fatal server error occurs.
func (a *App) Run(ctx context.Context) error {
	// Run orchestrates distinct startup steps.
	if err := a.setupKMS(ctx); err != nil {
		return err
	}

	a.printBanner()

	errCh := a.startHTTP(ctx)

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// setupKMS starts and registers KMS when encryption is enabled.
func (a *App) setupKMS(ctx context.Context) error {
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
			return fmt.Errorf("failed to determine executable path: %w", err)
		}
		bin = filepath.Join(filepath.Dir(exePath), "kms")
	}

	useEnc := a.eff.Config.Security.Encryption.Use
	if ev := strings.TrimSpace(os.Getenv("PROGRESSDB_USE_ENCRYPTION")); ev != "" {
		switch strings.ToLower(ev) {
		case "1", "true", "yes":
			useEnc = true
		default:
			useEnc = false
		}
	}

	if !useEnc {
		log.Printf("encryption enabled: false")
		return nil
	}

	// master key selection
	var mk string
	switch {
	case strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyFile) != "":
		mkFile := strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyFile)
		keyb, err := os.ReadFile(mkFile)
		if err != nil {
			return fmt.Errorf("failed to read master key file %s: %w", mkFile, err)
		}
		mk = strings.TrimSpace(string(keyb))
	case strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyHex) != "":
		mk = strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyHex)
	default:
		return fmt.Errorf("PROGRESSDB_USE_ENCRYPTION=true but no master key provided in server config. Set security.kms.master_key_file or security.kms.master_key_hex")
	}
	if mk == "" {
		return fmt.Errorf("master key is empty")
	}
	if kb, err := hex.DecodeString(mk); err != nil || len(kb) != 32 {
		return fmt.Errorf("invalid master_key_hex: must be 64-hex (32 bytes)")
	}

	// write launcher config
	lcfg := &kms.LauncherConfig{MasterKeyHex: mk, Socket: socket, DataDir: dataDir}
	kmsCfgPath, err := kms.CreateSecureConfigFile(lcfg, dataDir)
	if err != nil {
		return fmt.Errorf("failed to write kms config: %w", err)
	}

	// prebind socket
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

	h, err := kms.StartChildLauncher(ctx, bin, kmsCfgPath, ln)
	if parentListenerClose != nil {
		parentListenerClose()
	}
	if err != nil {
		return fmt.Errorf("failed to start KMS: %w", err)
	}
	a.child = &kms.CmdHandle{Cmd: h.Cmd}
	a.rc = kms.NewRemoteClient(socket)
	security.RegisterKMSProvider(a.rc)
	if err := a.rc.Health(); err != nil {
		return fmt.Errorf("KMS health check failed at %s: %w; ensure KMS is installed and reachable", socket, err)
	}

	kctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	go func() { <-kctx.Done() }()
	log.Printf("encryption enabled: true (KMS socket=%s)", socket)
	return nil
}

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
	banner.Print(a.eff.Addr, a.eff.DBPath, strings.Join(srcs, ", "), verStr)
}

// startHTTP builds the handler, starts the HTTP server in a goroutine and
// returns a channel that will contain any server error.
func (a *App) startHTTP(ctx context.Context) <-chan error {
	mux := http.NewServeMux()
	mux.Handle("/viewer/", http.StripPrefix("/viewer/", http.FileServer(http.Dir("./viewer"))))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ok\"}"))
	})
	mux.Handle("/", api.Handler())
	mux.Handle("/docs/", httpSwagger.Handler(httpSwagger.URL("/openapi.yaml")))
	mux.Handle("/openapi.yaml", http.FileServer(http.Dir("./docs")))
	mux.Handle("/metrics", promhttp.Handler())

	secCfg := security.SecConfig{
		AllowedOrigins: append([]string{}, a.eff.Config.Security.CORS.AllowedOrigins...),
		RPS:            a.eff.Config.Security.RateLimit.RPS,
		Burst:          a.eff.Config.Security.RateLimit.Burst,
		IPWhitelist:    append([]string{}, a.eff.Config.Security.IPWhitelist...),
		BackendKeys:    map[string]struct{}{},
		FrontendKeys:   map[string]struct{}{},
		AdminKeys:      map[string]struct{}{},
	}
	for _, k := range a.eff.Config.Security.APIKeys.Backend {
		secCfg.BackendKeys[k] = struct{}{}
	}
	for _, k := range a.eff.Config.Security.APIKeys.Frontend {
		secCfg.FrontendKeys[k] = struct{}{}
	}
	for _, k := range a.eff.Config.Security.APIKeys.Admin {
		secCfg.AdminKeys[k] = struct{}{}
	}

	runtimeCfg := &config.RuntimeConfig{BackendKeys: map[string]struct{}{}, SigningKeys: map[string]struct{}{}}
	for _, k := range a.eff.Config.Security.APIKeys.Backend {
		runtimeCfg.BackendKeys[k] = struct{}{}
		runtimeCfg.SigningKeys[k] = struct{}{}
	}
	config.SetRuntime(runtimeCfg)

	wrapped := security.AuthenticateRequestMiddleware(secCfg)(mux)

	a.srv = &http.Server{Addr: a.eff.Addr, Handler: wrapped}

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

// Shutdown attempts to gracefully stop all running components.
func (a *App) Shutdown(ctx context.Context) error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.rc != nil {
		_ = a.rc.Close()
	}
	if a.child != nil {
		_ = a.child.Stop(5 * time.Second)
	}
	if a.srv != nil {
		ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = a.srv.Shutdown(ctx2)
	}
	return nil
}

// initFieldPolicy installs the encryption field policy from the effective
// config.
func initFieldPolicy(eff config.EffectiveConfigResult) error {
	var fieldSrc []config.FieldEntry
	switch {
	case len(eff.Config.Security.Encryption.Fields) > 0:
		fieldSrc = eff.Config.Security.Encryption.Fields
	case len(eff.Config.Security.Fields) > 0:
		fieldSrc = eff.Config.Security.Fields
	}
	if len(fieldSrc) == 0 {
		return nil
	}
	fields := make([]security.EncField, 0, len(fieldSrc))
	for _, f := range fieldSrc {
		fields = append(fields, security.EncField{Path: f.Path, Algorithm: f.Algorithm})
	}
	return security.SetFieldPolicy(fields)
}

// initValidation builds validation rules from config and sets them globally.
func initValidation(eff config.EffectiveConfigResult) {
	vr := validation.Rules{Types: map[string]string{}, MaxLen: map[string]int{}, Enums: map[string][]string{}}
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
		vr.WhenThen = append(vr.WhenThen, validation.WhenThenRule{WhenPath: wt.When.Path, Equals: wt.When.Equals, ThenReq: append([]string{}, wt.Then.Required...)})
	}
	validation.SetRules(vr)
}
