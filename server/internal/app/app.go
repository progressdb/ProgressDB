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

	srv     *http.Server
	srvFast *fasthttp.Server
	state   string

	// ingest processor + monitor
	ingestProc          *ingest.Processor
	ingestMonitorCancel context.CancelFunc
	hwSensor            *sensor.Sensor
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

	// If both the application-level WAL and Pebble WAL are disabled the
	// system is effectively running in-memory only. This may be an
	// explicit choice (e.g., for tests) but deserves a prominent warning
	// with an estimate of the potential data-loss window and relevant
	// buffering knobs so operators can reason about risk.
	appWALEnabled := eff.Config.Ingest.Queue.WAL.Enabled
	pebbleWALDisabled := true
	if eff.Config.Ingest.Queue.WAL.DisablePebbleWAL != nil {
		pebbleWALDisabled = *eff.Config.Ingest.Queue.WAL.DisablePebbleWAL
	}
	if !appWALEnabled && pebbleWALDisabled {
		// Estimate a loss window from producer/consumer batching knobs.
		// Use processor flush interval and queue truncate interval as
		// conservative approximations of how long significant state may
		// remain only in memory.
		procFlush := eff.Config.Ingest.Processor.FlushInterval.Duration()
		truncate := eff.Config.Ingest.Queue.TruncateInterval.Duration()
		// Choose the larger duration as a simple conservative estimate.
		lossWindow := procFlush
		if truncate > lossWindow {
			lossWindow = truncate
		}
		// Compute a simple messages-at-risk estimate and bytes-at-risk using
		// a conservative default average message size of 1 KiB.
		queueCapacity := eff.Config.Ingest.Queue.Capacity
		procWorkers := eff.Config.Ingest.Processor.Workers
		procBatch := eff.Config.Ingest.Processor.MaxBatchMsgs
		messagesAtRisk := queueCapacity + procWorkers*procBatch
		// bytesAtRisk := int64(messagesAtRisk) * 1024 // 1 KiB per message

		// Prepare human-friendly representations for key values.
		lossWindowHuman := lossWindow.String()
		processorFlushHuman := procFlush.String()
		truncateHuman := truncate.String()
		queueCapacityHuman := humanize.Comma(int64(queueCapacity))
		messagesAtRiskHuman := humanize.Comma(int64(messagesAtRisk))
		// bytesAtRiskHuman := humanize.Bytes(uint64(bytesAtRisk))

		// Log configuration summary
		summaryItems := []string{
			fmt.Sprintf("loss_window: %s", lossWindowHuman),
			fmt.Sprintf("processor_flush: %s", processorFlushHuman),
			fmt.Sprintf("queue_truncate: %s", truncateHuman),
			fmt.Sprintf("queue_capacity: %s", queueCapacityHuman),
			fmt.Sprintf("processor_workers: %d", procWorkers),
			fmt.Sprintf("processor_max_batch_msgs: %s", humanize.Comma(int64(procBatch))),
			fmt.Sprintf("messages_at_risk: %s", messagesAtRiskHuman),
			// fmt.Sprintf("bytes_at_risk: %s", bytesAtRiskHuman),
		}
		logger.LogConfigSummary("config_durability_summary", summaryItems)
	}

	// apply telemetry defaults from effective config
	telemetry.SetSampleRate(eff.Config.Telemetry.SampleRate)
	telemetry.SetSlowThreshold(eff.Config.Telemetry.SlowThreshold.Duration())

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

	// open store under <DBPath>/store (main ensures directories exist)
	if state.PathsVar.Store == "" {
		return nil, fmt.Errorf("state paths not initialized")
	}
	// pass configured pebble WAL disable flag
	// Default to the config's DisablePebbleWAL if provided, otherwise
	// preserve historical default (true).
	disable := true
	if aCfg := eff.Config; aCfg != nil {
		if aCfg.Ingest.Queue.WAL.DisablePebbleWAL != nil {
			disable = *aCfg.Ingest.Queue.WAL.DisablePebbleWAL
		}
	}
	if err := store.Open(state.PathsVar.Store, disable, appWALEnabled); err != nil {
		return nil, fmt.Errorf("failed to open pebble at %s: %w", state.PathsVar.Store, err)
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

	// start ingest processor

	// try to enable durable queuer
	// if it fails, fallback to inmemory queuer
	deOpts := queue.DurableEnableOptions{
		Dir:               state.WalPath(a.eff.DBPath),
		Capacity:          a.eff.Config.Ingest.Queue.Capacity,
		TruncateInterval:  a.eff.Config.Ingest.Queue.TruncateInterval.Duration(),
		WALMaxFileSize:    a.eff.Config.Ingest.Queue.WAL.MaxFileSize.Int64(),
		WALEnableBatch:    a.eff.Config.Ingest.Queue.WAL.EnableBatch,
		WALBatchSize:      a.eff.Config.Ingest.Queue.WAL.BatchSize,
		WALBatchInterval:  a.eff.Config.Ingest.Queue.WAL.BatchInterval.Duration(),
		WALEnableCompress: a.eff.Config.Ingest.Queue.WAL.EnableCompress,
	}
	if err := queue.EnableDurable(deOpts); err != nil {
		// fallback to in-memory queue constructed from config
		q := queue.NewQueueFromConfig(a.eff.Config.Ingest.Queue)
		queue.SetDefaultQueue(q)
	}

	// Ensure DefaultQueue is set by now (either durable or in-memory)
	p := ingest.NewProcessor(queue.DefaultQueue, a.eff.Config.Ingest.Processor)
	ingest.RegisterDefaultHandlers(p)
	p.Start()
	a.ingestProc = p

	// start pebble monitor
	// convert effective config monitor to sensor.MonitorConfig
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
		// stop receiving queues
		if queue.DefaultQueue != nil {
			queue.DefaultQueue.Close()
		}

		// close durable WAL if enabled
		_ = queue.CloseDurable()

		// stop processing new things
		if a.ingestProc != nil {
			a.ingestProc.Stop(context.Background())
		}

		// stop the sensors
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
