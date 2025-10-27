package app

import (
	"context"
	"fmt"

	"github.com/joho/godotenv"
	"github.com/valyala/fasthttp"

	"progressdb/pkg/ingest"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
	"progressdb/pkg/sensor"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/telemetry"

	"progressdb/internal/retention"
	"progressdb/pkg/config"
	"progressdb/pkg/kms"
	"progressdb/pkg/security"
	"progressdb/pkg/state"
)

// app groups server state and components.
type App struct {
	retentionCancel context.CancelFunc
	eff             config.EffectiveConfigResult
	version         string
	commit          string
	buildDate       string
	rc              *kms.RemoteClient // kms
	cancel          context.CancelFunc

	srvFast *fasthttp.Server
	state   string

	ingestIngestor *ingest.Ingestor
	hwSensor       *sensor.Sensor
}

// new sets up resources that don't need a running context (db, validation, runtime keys, etc).
// does not start kms or http server. call run to start those and block for lifecycle.
func New(eff config.EffectiveConfigResult, version, commit, buildDate string) (*App, error) {
	_ = godotenv.Load(".env")

	// validate config and fail fast if not valid
	if err := validateConfig(eff); err != nil {
		return nil, err
	}

	// open store (caller ensures directories exist)
	if state.PathsVar.Store == "" {
		return nil, fmt.Errorf("state paths not initialized")
	}
	disable := true // always disable Pebble WAL since we have app-level WAL
	appWALenabled := eff.Config.Ingest.Intake.WAL.Enabled
	if err := storedb.Open(state.PathsVar.Store, disable, appWALenabled); err != nil {
		return nil, fmt.Errorf("failed to open pebble at %s: %w", state.PathsVar.Store, err)
	}
	logger.Info("database_opened", "path", state.PathsVar.Store)

	// warn if WAL is disabled
	if !appWALenabled {
		logger.Warn("wal_disabled_data_risk", "message", "WAL is disabled - potential data loss during crash")
	}

	// telemetry setup
	telemetry.InitWithStrategy(
		state.PathsVar.Tel,
		int(eff.Config.Telemetry.BufferSize.Int64()),
		eff.Config.Telemetry.QueueCapacity,
		eff.Config.Telemetry.FlushInterval.Duration(),
		eff.Config.Telemetry.FileMaxSize.Int64(),
		telemetry.RotationStrategyPurge,
	)

	// setup runtime keys
	runtimeCfg := &config.RuntimeConfig{BackendKeys: map[string]struct{}{}, SigningKeys: map[string]struct{}{}}
	for _, k := range eff.Config.Server.APIKeys.Backend {
		runtimeCfg.BackendKeys[k] = struct{}{}
		runtimeCfg.SigningKeys[k] = struct{}{}
	}
	config.SetRuntime(runtimeCfg)

	// set up encryption field policy
	if err := initFieldPolicy(eff); err != nil {
		return nil, fmt.Errorf("invalid encryption fields: %w", err)
	}

	a := &App{eff: eff, version: version, commit: commit, buildDate: buildDate}
	return a, nil
}

// run starts kms (if enabled), http server, and blocks until context cancellation or fatal error.
func (a *App) Run(ctx context.Context) error {
	if err := a.setupKMS(ctx); err != nil {
		return err
	}
	a.printBanner()

	// open database
	disablePebbleWAL := true // always disable Pebble WAL
	appWALEnabled := a.eff.Config.Ingest.Intake.WAL.Enabled
	logger.Info("opening_database", "path", state.PathsVar.Store, "disable_pebble_wal", disablePebbleWAL, "app_wal_enabled", appWALEnabled)
	if err := storedb.Open(state.PathsVar.Store, disablePebbleWAL, appWALEnabled); err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	logger.Info("database_opened", "path", state.PathsVar.Store)

	// open index database
	logger.Info("opening_index", "path", state.PathsVar.Index, "disable_pebble_wal", disablePebbleWAL, "app_wal_enabled", appWALEnabled)
	if err := index.Open(state.PathsVar.Index, disablePebbleWAL, appWALEnabled); err != nil {
		return fmt.Errorf("failed to open index: %w", err)
	}
	logger.Info("index_opened", "path", state.PathsVar.Index)

	// initialize recovery system (will run after queue is created)
	recoveryConfig := a.eff.Config.Ingest.Intake.Recovery
	ingest.InitGlobalRecovery(
		nil, // queue will be set after queue initialization
		storedb.Client,
		index.IndexDB,
		recoveryConfig.Enabled,
		recoveryConfig.WALEnabled && appWALEnabled,
		recoveryConfig.TempIdxEnabled,
	)

	// start retention scheduler if enabled
	retention.SetEffectiveConfig(a.eff)
	if cancel, err := retention.Start(ctx, a.eff); err != nil {
		return err
	} else {
		a.retentionCancel = cancel
	}

	// init intake queue
	if err := queue.InitGlobalIngestQueue(a.eff.Config.Ingest.Intake, a.eff.DBPath); err != nil {
		return fmt.Errorf("failed to init queue: %w", err)
	}

	// set queue for recovery system
	ingest.SetRecoveryQueue(queue.GlobalIngestQueue)

	// run crash recovery before starting ingestor
	recoveryStats := ingest.RunGlobalRecovery()
	if recoveryStats.WALErrors > 0 || recoveryStats.TempIndexErrors > 0 {
		logger.Warn("recovery_completed_with_errors",
			"wal_errors", recoveryStats.WALErrors,
			"temp_index_errors", recoveryStats.TempIndexErrors)
	}

	ingestor := ingest.NewIngestor(queue.GlobalIngestQueue, a.eff.Config.Ingest.Compute, a.eff.Config.Ingest.Apply, a.eff.DBPath)
	ingestor.Start()
	a.ingestIngestor = ingestor

	// start hardware sensor
	mon := sensor.MonitorConfig{
		PollInterval:   a.eff.Config.Sensor.Monitor.PollInterval.Duration(),
		DiskHighPct:    a.eff.Config.Sensor.Monitor.DiskHighPct,
		DiskLowPct:     a.eff.Config.Sensor.Monitor.DiskLowPct,
		MemHighPct:     a.eff.Config.Sensor.Monitor.MemHighPct,
		CPUHighPct:     a.eff.Config.Sensor.Monitor.CPUHighPct,
		RecoveryWindow: a.eff.Config.Sensor.Monitor.RecoveryWindow.Duration(),
	}
	sensorObj := sensor.NewSensor(mon)
	sensorObj.Start()
	a.hwSensor = sensorObj

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
	fieldPaths := eff.Config.Encryption.Fields
	if len(fieldPaths) == 0 {
		return nil
	}
	fields := make([]string, 0, len(fieldPaths))
	for _, path := range fieldPaths {
		fields = append(fields, path)
	}
	return security.SetEncryptionFieldPolicy(fields)
}
