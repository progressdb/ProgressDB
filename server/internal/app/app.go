package app

import (
	"context"
	"fmt"

	"github.com/joho/godotenv"

	"net/http"

	"progressdb/internal/retention"
	"progressdb/pkg/config"
	"progressdb/pkg/kms"
	"progressdb/pkg/security"
	"progressdb/pkg/store"
)

// App encapsulates the server components and lifecycle.
type App struct {
	retentionCancel context.CancelFunc
	eff             config.EffectiveConfigResult
	version         string
	commit          string
	buildDate       string

	// KMS/runtime
	rc     *kms.RemoteClient
	cancel context.CancelFunc

	srv   *http.Server
	state string
}

// New initializes resources that do not require a running context (DB,
// validation, field policy, runtime keys). It does not start KMS or the
// HTTP server; call Run to start those and block until shutdown.
func New(eff config.EffectiveConfigResult, version, commit, buildDate string) (*App, error) {
	_ = godotenv.Load(".env")

	// validate effective config early and fail fast
	if err := validateConfig(eff); err != nil {
		return nil, err
	}

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
	// initValidation(eff)

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
	// run the kms service - depending on config
	if err := a.setupKMS(ctx); err != nil {
		return err
	}

	// print banner
	a.printBanner()

	// start retention scheduler if enabled
	// register effective config so tests may trigger runs
	retention.SetEffectiveConfig(a.eff)
	if cancel, err := retention.Start(ctx, a.eff); err != nil {
		return err
	} else {
		a.retentionCancel = cancel
	}

	// start the http server
	errCh := a.startHTTP(ctx)

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// initFieldPolicy installs the encryption field policy from the effective config
func initFieldPolicy(eff config.EffectiveConfigResult) error {
	// The config.Security.Encryption.Fields is now []string (field paths)
	fieldPaths := eff.Config.Security.Encryption.Fields
	if len(fieldPaths) == 0 {
		return nil
	}
	fields := make([]string, 0, len(fieldPaths))
	for _, path := range fieldPaths {
		fields = append(fields, path)
	}
	return security.SetEncryptionFieldPolicy(fields)
}

// initValidation builds validation rules from config and sets them globally.
// func initValidation(eff config.EffectiveConfigResult) {
// 	vr := validation.Rules{Types: map[string]string{}, MaxLen: map[string]int{}, Enums: map[string][]string{}}
// 	vr.Required = append(vr.Required, eff.Config.Validation.Required...)
// 	for _, t := range eff.Config.Validation.Types {
// 		vr.Types[t.Path] = t.Type
// 	}
// 	for _, ml := range eff.Config.Validation.MaxLen {
// 		vr.MaxLen[ml.Path] = ml.Max
// 	}
// 	for _, e := range eff.Config.Validation.Enums {
// 		vr.Enums[e.Path] = append([]string{}, e.Values...)
// 	}
// 	for _, wt := range eff.Config.Validation.WhenThen {
// 		vr.WhenThen = append(vr.WhenThen, validation.WhenThenRule{WhenPath: wt.When.Path, Equals: wt.When.Equals, ThenReq: append([]string{}, wt.Then.Required...)})
// 	}
// 	validation.SetRules(vr)
// }
