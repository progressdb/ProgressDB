package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/joho/godotenv"
	"github.com/valyala/fasthttp"

	"progressdb/pkg/ingest"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
	"progressdb/pkg/sensor"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/telemetry"

	"github.com/dustin/go-humanize"

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

	srv     *http.Server
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

	// warn if both wals are disabled and summarize potential data loss window
	appWALenabled := eff.Config.Ingest.Queue.Durable.RecoverOnStartup
	pebbleWALdisabled := true
	if eff.Config.Ingest.Queue.Durable.DisablePebbleWAL != nil {
		pebbleWALdisabled = *eff.Config.Ingest.Queue.Durable.DisablePebbleWAL
	}
	if !appWALenabled && pebbleWALdisabled {
		var flushMs int
		if eff.Config.Ingest.Queue.Mode == "memory" {
			flushMs = eff.Config.Ingest.Queue.Memory.FlushIntervalMs
		} else {
			flushMs = eff.Config.Ingest.Queue.Durable.FlushIntervalMs
		}
		procFlush := time.Duration(flushMs) * time.Millisecond
		lossWindow := procFlush
		queueCapacity := eff.Config.Ingest.Queue.BufferCapacity
		procWorkers := eff.Config.Ingest.Ingestor.WorkerCount
		var procBatch int
		if eff.Config.Ingest.Queue.Mode == "memory" {
			procBatch = eff.Config.Ingest.Queue.Memory.FlushBatchSize
		} else {
			procBatch = eff.Config.Ingest.Queue.Durable.FlushBatchSize
		}
		messagesAtRisk := queueCapacity + procWorkers*procBatch

		lossWindowHuman := lossWindow.String()
		processorFlushHuman := procFlush.String()
		queueCapacityHuman := humanize.Comma(int64(queueCapacity))
		messagesAtRiskHuman := humanize.Comma(int64(messagesAtRisk))

		summaryItems := []string{
			fmt.Sprintf("loss_window: %s", lossWindowHuman),
			fmt.Sprintf("processor_flush: %s", processorFlushHuman),
			fmt.Sprintf("queue_capacity: %s", queueCapacityHuman),
			fmt.Sprintf("processor_workers: %d", procWorkers),
			fmt.Sprintf("processor_max_batch_msgs: %s", humanize.Comma(int64(procBatch))),
			fmt.Sprintf("messages_at_risk: %s", messagesAtRiskHuman),
		}
		logger.LogConfigSummary("config_durability_summary", summaryItems)
	}

	// telemetry setup
	telemetry.Init(
		state.PathsVar.Tel,
		int(eff.Config.Telemetry.BufferSize.Int64()),
		eff.Config.Telemetry.QueueCapacity,
		eff.Config.Telemetry.FlushInterval.Duration(),
		eff.Config.Telemetry.FileMaxSize.Int64(),
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

	// open store (caller ensures directories exist)
	if state.PathsVar.Store == "" {
		return nil, fmt.Errorf("state paths not initialized")
	}
	disable := true
	if aCfg := eff.Config; aCfg != nil {
		if aCfg.Ingest.Queue.Durable.DisablePebbleWAL != nil {
			disable = *aCfg.Ingest.Queue.Durable.DisablePebbleWAL
		}
	}
	if err := storedb.Open(state.PathsVar.Store, disable, appWALenabled); err != nil {
		return nil, fmt.Errorf("failed to open pebble at %s: %w", state.PathsVar.Store, err)
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

	// start retention scheduler if enabled
	retention.SetEffectiveConfig(a.eff)
	if cancel, err := retention.Start(ctx, a.eff); err != nil {
		return err
	} else {
		a.retentionCancel = cancel
	}

	// open database
	disablePebbleWAL := true
	if a.eff.Config.Ingest.Queue.Durable.DisablePebbleWAL != nil {
		disablePebbleWAL = *a.eff.Config.Ingest.Queue.Durable.DisablePebbleWAL
	}
	appWALEnabled := a.eff.Config.Ingest.Queue.Mode == "durable"
	logger.Info("opening_database", "path", a.eff.DBPath, "disable_pebble_wal", disablePebbleWAL, "app_wal_enabled", appWALEnabled)
	if err := storedb.Open(a.eff.DBPath, disablePebbleWAL, appWALEnabled); err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	logger.Info("database_opened", "path", a.eff.DBPath)

	// config based basqueue
	q, err := queue.NewQueueFromConfig(a.eff.Config.Ingest.Queue, a.eff.DBPath)
	if err != nil {
		return fmt.Errorf("failed to create queue: %w", err)
	}
	queue.SetDefaultIngestQueue(q)
	ingestor := ingest.NewIngestor(queue.DefaultIngestQueue, a.eff.Config.Ingest.Ingestor, a.eff.Config.Ingest.Queue)
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
		// shutdown HTTP server
		if a.srvFast != nil {
			_ = a.srvFast.Shutdown()
		}

		// shutdown ingest and sensor
		if queue.DefaultIngestQueue != nil {
			_ = queue.DefaultIngestQueue.Close()
		}

		// stop ingestor
		if a.ingestIngestor != nil {
			a.ingestIngestor.Stop(context.Background())
		}

		// stop sensor
		if a.hwSensor != nil {
			a.hwSensor.Stop()
		}

		// close tel
		telemetry.Close()
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
