package app

import (
	"context"
	"fmt"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/ingest"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/state/sensor"
	"progressdb/pkg/state/telemetry"
	indexdb "progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"

	"progressdb/internal/retention"
	"progressdb/pkg/config"
	"progressdb/pkg/state"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/encryption/kms"
)

type App struct {
	retentionCancel context.CancelFunc
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
func New(version, commit, buildDate string) (*App, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("config not set - call config.SetConfig() first")
	}

	// telemetry setup
	telemetry.InitWithStrategy(
		state.PathsVar.Tel,
		int(cfg.Telemetry.BufferSize.Int64()),
		cfg.Telemetry.QueueCapacity,
		cfg.Telemetry.FlushInterval.Duration(),
		cfg.Telemetry.FileMaxSize.Int64(),
		telemetry.RotationStrategyPurge,
	)

	// setup runtime keys
	runtimeCfg := &config.RuntimeConfig{
		BackendKeys:    map[string]struct{}{},
		SigningKeys:    map[string]struct{}{},
		MaxPayloadSize: cfg.Server.MaxPayloadSize.Int64(),
	}
	for _, k := range cfg.Server.APIKeys.Backend {
		runtimeCfg.BackendKeys[k] = struct{}{}
		runtimeCfg.SigningKeys[k] = struct{}{}
	}
	config.SetRuntime(runtimeCfg)

	// set up encryption field policy
	if err := initFieldPolicy(); err != nil {
		return nil, fmt.Errorf("invalid encryption fields: %w", err)
	}

	a := &App{version: version, commit: commit, buildDate: buildDate}
	return a, nil
}

func (a *App) Run(ctx context.Context) error {
	// establish kms setup
	if err := a.setupKMS(ctx); err != nil {
		return err
	}
	a.printBanner()

	// open database
	cfg := config.GetConfig()
	storageWalEnabled := cfg.Storage.WAL
	appWALEnabled := cfg.Ingest.Intake.WAL.Enabled
	logger.Info("opening_database", "path", state.PathsVar.Store, "disable_pebble_wal", storageWalEnabled, "app_wal_enabled", appWALEnabled)

	if state.PathsVar.Store == "" || state.PathsVar.Index == "" {
		return fmt.Errorf("state paths not initialized")
	}

	// open storedb
	if err := storedb.Open(state.PathsVar.Store, storageWalEnabled, appWALEnabled); err != nil {
		return fmt.Errorf("failed to open pebble at %s: %w", state.PathsVar.Store, err)
	}
	logger.Info("database_opened", "path", state.PathsVar.Store)

	// open indexdb
	if err := indexdb.Open(state.PathsVar.Index, storageWalEnabled, appWALEnabled); err != nil {
		return fmt.Errorf("failed to open pebble at %s: %w", state.PathsVar.Index, err)
	}
	logger.Info("database_opened", "path", state.PathsVar.Index)

	// warn if WAL is disabled
	if !appWALEnabled {
		logger.Warn("wal_disabled_data_risk", "message", "WAL is disabled - potential data loss during crash")
	}

	// initialize recovery system (will run after queue is created)
	recoveryConfig := cfg.Ingest.Intake.Recovery
	ingest.InitGlobalRecovery(
		nil, // queue will be set after queue initialization
		storedb.Client,
		indexdb.Client,
		recoveryConfig.Enabled,
		recoveryConfig.WALEnabled && appWALEnabled,
		recoveryConfig.TempIdxEnabled,
	)

	// start retention scheduler if enabled
	if cancel, err := retention.Start(ctx); err != nil {
		return err
	} else {
		a.retentionCancel = cancel
	}

	// init intake queue
	if err := queue.InitGlobalIngestQueue(cfg.Ingest.Intake, cfg.Server.DBPath); err != nil {
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

	ingestor := ingest.NewIngestor(queue.GlobalIngestQueue, cfg.Ingest.Compute, cfg.Ingest.Apply, cfg.Server.DBPath)
	ingestor.Start()
	a.ingestIngestor = ingestor

	// start hardware sensor
	mon := sensor.MonitorConfig{
		PollInterval:   cfg.Sensor.Monitor.PollInterval.Duration(),
		DiskHighPct:    cfg.Sensor.Monitor.DiskHighPct,
		DiskLowPct:     cfg.Sensor.Monitor.DiskLowPct,
		MemHighPct:     cfg.Sensor.Monitor.MemHighPct,
		CPUHighPct:     cfg.Sensor.Monitor.CPUHighPct,
		RecoveryWindow: cfg.Sensor.Monitor.RecoveryWindow.Duration(),
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

// sets into state
func initFieldPolicy() error {
	cfg := config.GetConfig()
	fieldPaths := cfg.Encryption.Fields
	if len(fieldPaths) == 0 {
		return nil
	}
	fields := make([]string, 0, len(fieldPaths))
	for _, path := range fieldPaths {
		fields = append(fields, path)
	}
	return encryption.SetEncryptionFieldPolicy(fields)
}
