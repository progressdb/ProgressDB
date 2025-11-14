package app

import (
	"context"
	"fmt"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/ingest"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/tracking"
	"progressdb/pkg/ingest/wally"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/state/sensor"
	"progressdb/pkg/state/telemetry"
	indexdb "progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/migrations"

	"progressdb/internal/retention"
	"progressdb/pkg/config"
	"progressdb/pkg/state"
	"progressdb/pkg/store/encryption"
)

type App struct {
	retentionCancel context.CancelFunc
	version         string
	commit          string
	buildDate       string

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
		return nil, fmt.Errorf("config not set - call config.SetConfig() first. Make sure you're running the service through the main.go entry point with proper config file")
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
	}
	for _, k := range cfg.Server.APIKeys.Signing {
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
	if err := encryption.SetupKMS(ctx); err != nil {
		return err
	}
	a.printBanner()

	// open database
	cfg := config.GetConfig()
	intakeWALEnabled := cfg.Ingest.Intake.WAL.Enabled
	logger.Info(
		"opening_database",
		"store_path", state.PathsVar.Store,
		"index_path", state.PathsVar.Index,
	)
	logger.Info(
		"wal_settings",
		"intake_wal_enabled", intakeWALEnabled,
	)

	// paths check
	if state.PathsVar.Store == "" || state.PathsVar.Index == "" {
		return fmt.Errorf("state paths not initialized")
	}

	// durability check
	if !intakeWALEnabled {
		logger.Warn(
			"intake_wal_disabled",
			"Intake WAL disabled - storage WAL always enabled for durability",
		)
	}

	// open storedb
	if err := storedb.Open(state.PathsVar.Store, intakeWALEnabled); err != nil {
		return fmt.Errorf("failed to open pebble at %s: %w", state.PathsVar.Store, err)
	}
	logger.Info("database_opened", "path", state.PathsVar.Store)

	// open indexdb
	if err := indexdb.Open(state.PathsVar.Index, intakeWALEnabled); err != nil {
		return fmt.Errorf("failed to open pebble at %s: %w", state.PathsVar.Index, err)
	}
	logger.Info("database_opened", "path", state.PathsVar.Index)

	// run version checks and migrations after databases are opened
	if _, err := migrations.Run(ctx, a.version); err != nil {
		return fmt.Errorf("migrations run failed: %w", err)
	}

	// start retention scheduler if enabled
	if cancel, err := retention.Start(ctx); err != nil {
		return err
	} else {
		a.retentionCancel = cancel
	}

	// init intake queue
	if err := queue.InitGlobalIngestQueue(cfg.Server.DBPath); err != nil {
		return fmt.Errorf("failed to init queue: %w", err)
	}

	// init in-flight tracking
	tracking.InitGlobalInflightTracker()

	// init key mapper
	tracking.InitGlobalKeyMapper()

	// initialize WAL replay system with queue
	wally.InitWALReplay(queue.GlobalIngestQueue)

	// run crash replay before starting ingestor
	wally.ReplayWAL()

	// start queue & others
	ingestor := ingest.NewIngestor(queue.GlobalIngestQueue, cfg.Server.DBPath)
	ingestor.Start()
	a.ingestIngestor = ingestor

	// start hardware sensor
	sensor := sensor.NewSensorFromConfig()
	sensor.Start()
	a.hwSensor = sensor

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
