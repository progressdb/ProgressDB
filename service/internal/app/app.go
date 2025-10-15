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
	"progressdb/pkg/telemetry"

	"github.com/dustin/go-humanize"

	"progressdb/internal/retention"
	"progressdb/pkg/config"
	"progressdb/pkg/kms"
	"progressdb/pkg/security"
	"progressdb/pkg/state"
	"progressdb/pkg/store"
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

	ingestProc          *ingest.Processor
	ingestMonitorCancel context.CancelFunc
	hwSensor            *sensor.Sensor
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
	appWALenabled := eff.Config.Ingest.Queue.Durable.Recover
	pebbleWALdisabled := true
	if eff.Config.Ingest.Queue.Durable.DisablePebbleWAL != nil {
		pebbleWALdisabled = *eff.Config.Ingest.Queue.Durable.DisablePebbleWAL
	}
	if !appWALenabled && pebbleWALdisabled {
		procFlush := time.Duration(eff.Config.Ingest.Processor.FlushMs) * time.Millisecond
		truncate := eff.Config.Ingest.Queue.Durable.TruncateInterval.Duration()
		lossWindow := procFlush
		if truncate > lossWindow {
			lossWindow = truncate
		}
		queueCapacity := eff.Config.Ingest.Queue.Capacity
		procWorkers := eff.Config.Ingest.Processor.Workers
		procBatch := eff.Config.Ingest.Processor.MaxBatchMsgs
		messagesAtRisk := queueCapacity + procWorkers*procBatch

		lossWindowHuman := lossWindow.String()
		processorFlushHuman := procFlush.String()
		truncateHuman := truncate.String()
		queueCapacityHuman := humanize.Comma(int64(queueCapacity))
		messagesAtRiskHuman := humanize.Comma(int64(messagesAtRisk))

		summaryItems := []string{
			fmt.Sprintf("loss_window: %s", lossWindowHuman),
			fmt.Sprintf("processor_flush: %s", processorFlushHuman),
			fmt.Sprintf("queue_truncate: %s", truncateHuman),
			fmt.Sprintf("queue_capacity: %s", queueCapacityHuman),
			fmt.Sprintf("processor_workers: %d", procWorkers),
			fmt.Sprintf("processor_max_batch_msgs: %s", humanize.Comma(int64(procBatch))),
			fmt.Sprintf("messages_at_risk: %s", messagesAtRiskHuman),
		}
		logger.LogConfigSummary("config_durability_summary", summaryItems)
	}

	// telemetry defaults
	telemetry.SetSampleRate(eff.Config.Telemetry.SampleRate)
	telemetry.SetSlowThreshold(eff.Config.Telemetry.SlowThreshold.Duration())

	// setup runtime keys
	runtimeCfg := &config.RuntimeConfig{BackendKeys: map[string]struct{}{}, SigningKeys: map[string]struct{}{}}
	for _, k := range eff.Config.Security.APIKeys.Backend {
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
	if err := store.Open(state.PathsVar.Store, disable, appWALenabled); err != nil {
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

	// start hardware sensor
	sensorObj := sensor.NewSensor(500 * time.Millisecond)
	sensorObj.Start()
	a.hwSensor = sensorObj

	// ensure defaultqueue is set (either durable or in-memory)
	p := ingest.NewProcessor(queue.DefaultIngestQueue, a.eff.Config.Ingest.Processor)
	ingest.RegisterDefaultHandlers(p)
	p.Start()
	a.ingestProc = p

	// start pebble monitor
	mon := sensor.MonitorConfig{
		PollInterval:   a.eff.Config.Sensor.Monitor.PollInterval.Duration(),
		WALHighBytes:   uint64(a.eff.Config.Sensor.Monitor.WALHighBytes.Int64()),
		WALLowBytes:    uint64(a.eff.Config.Sensor.Monitor.WALLowBytes.Int64()),
		DiskHighPct:    a.eff.Config.Sensor.Monitor.DiskHighPct,
		DiskLowPct:     a.eff.Config.Sensor.Monitor.DiskLowPct,
		RecoveryWindow: a.eff.Config.Sensor.Monitor.RecoveryWindow.Duration(),
	}
	cancelMonitor := sensor.StartPebbleMonitor(ctx, p, sensorObj, mon)
	a.ingestMonitorCancel = cancelMonitor

	errCh := a.startHTTP(ctx)

	select {
	case <-ctx.Done():
		// shutdown ingest and sensor
		if a.ingestMonitorCancel != nil {
			a.ingestMonitorCancel()
		}
		if queue.DefaultBackend() != nil {
			queue.DefaultBackend().Close()
		}
		_ = queue.CloseDurable()

		if a.ingestProc != nil {
			a.ingestProc.Stop(context.Background())
		}
		if a.hwSensor != nil {
			a.hwSensor.Stop()
		}
		return nil
	case err := <-errCh:
		return err
	}
}

// initFieldPolicy installs the encryption field policy from the effective config
func initFieldPolicy(eff config.EffectiveConfigResult) error {
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
