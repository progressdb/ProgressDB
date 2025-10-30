package retention

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/adhocore/gronx"

	"progressdb/pkg/config"
	"progressdb/pkg/state"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/timeutil"
)

// TODO: Dont delete anything deleted in the last 4 hours.

// store the effective config for test or admin use
func SetEffectiveConfig(eff config.EffectiveConfigResult) {
}

// run retention immediately using the global config; error if not set
func RunImmediate() error {
	if config.GetConfig() == nil {
		return fmt.Errorf("no config set for retention run")
	}
	if state.PathsVar.Retention == "" {
		return fmt.Errorf("state paths not initialized")
	}
	retentionPath := state.PathsVar.Retention
	return runOnce(context.Background(), retentionPath)
}

// enable scheduled execution if configured; returns cancel function
func Start(ctx context.Context) (context.CancelFunc, error) {
	cfg := config.GetConfig()
	ret := cfg.Retention

	// not enabled, return a no-op cancel
	if !ret.Enabled {
		logger.Info("retention_disabled")
		return func() {}, nil
	}

	// use <DBPath>/state/retention for lock and artifacts
	retentionPath := state.PathsVar.Retention

	// ensure the directory exists
	if err := os.MkdirAll(retentionPath, 0o700); err != nil {
		logger.Error("retention_path_create_failed", "path", retentionPath, "error", err)
		return nil, err
	}

	// validate cron syntax
	cronExpr := ret.Cron
	logger.Info("retention_enabled", "cron", cronExpr, "period", ret.Period, "path", retentionPath)
	ctx2, cancel := context.WithCancel(ctx)

	// run the scheduler goroutine
	go runScheduler(ctx2, retentionPath, cronExpr)

	logger.Info("retention_scheduler_started", "path", retentionPath)
	return cancel, nil
}

// schedules and triggers based on the cron expression
func runScheduler(ctx context.Context, auditPath string, cronExpr string) {
	for {
		select {
		case <-ctx.Done():
			logger.Info("retention_scheduler_stopping")
			return
		default:
		}

		// calculate next tick after now (UTC)
		now := timeutil.Now()
		next, err := gronx.NextTickAfter(cronExpr, now, false)
		if err != nil {
			logger.Error("retention_nexttick_failed", "cron", cronExpr, "error", err)
			// fallback and retry after a short delay
			select {
			case <-time.After(30 * time.Second):
			case <-ctx.Done():
				logger.Info("retention_scheduler_stopping")
				return
			}
			continue
		}

		wait := time.Until(next)
		if wait <= 0 {
			// time is due, run immediately
			go func() {
				if err := runOnce(ctx, auditPath); err != nil {
					logger.Error("retention_run_error", "error", err)
				}
			}()
			// avoid a tight loop by sleeping briefly
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				logger.Info("retention_scheduler_stopping")
				return
			}
			continue
		}

		// wait until next tick or cancellation
		select {
		case <-time.After(wait):
			go func() {
				if err := runOnce(ctx, auditPath); err != nil {
					logger.Error("retention_run_error", "error", err)
				}
			}()
		case <-ctx.Done():
			logger.Info("retention_scheduler_stopping")
			return
		}
	}
}
