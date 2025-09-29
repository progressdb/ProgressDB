package retention

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adhocore/gronx"

	"progressdb/pkg/config"
	"progressdb/pkg/logger"
)

var storedEff *config.EffectiveConfigResult

// SetEffectiveConfig stores the effective config so tests (or admin triggers)
// can invoke retention runs on-demand. This is intended for testing only.
func SetEffectiveConfig(eff config.EffectiveConfigResult) {
	storedEff = &eff
}

// RunImmediate triggers a single retention run using the stored effective
// config. Returns an error if no effective config was registered.
func RunImmediate() error {
	if storedEff == nil {
		return fmt.Errorf("no effective config registered for retention run")
	}
	auditPath := filepath.Join(storedEff.DBPath, "retention")
	return runOnce(context.Background(), *storedEff, auditPath)
}

// Start starts the retention scheduler if enabled. Returns a cancel func.
func Start(ctx context.Context, eff config.EffectiveConfigResult) (context.CancelFunc, error) {
	ret := eff.Config.Retention

	// if retention is not enabled, return no-op cancel
	if !ret.Enabled {
		logger.Info("retention_disabled", nil)
		return func() {}, nil
	}

	// Use a stable retention folder under the DB path for lock and audit
	// artifacts: <DBPath>/retention.
	auditPath := filepath.Join(eff.DBPath, "retention")

	// ensure audit path exists
	if err := os.MkdirAll(auditPath, 0o700); err != nil {
		logger.Error("retention_audit_path_create_failed", "path", auditPath, "error", err)
		return nil, err
	}

	// note: audit sinks are configured externally; retention simply emits
	// audit events via the global logger.

	// map empty cron to default daily @02:00
	cronExpr := ret.Cron
	if cronExpr == "" {
		cronExpr = "0 2 * * *"
	}
	// validate cron expression using gronx
	if !gronx.IsValid(cronExpr) {
		logger.Error("retention_invalid_cron", "cron", ret.Cron)
		return nil, fmt.Errorf("invalid retention cron expression: %s", ret.Cron)
	}

	logger.Info("retention_enabled", "cron", cronExpr, "period", ret.Period, "audit_path", auditPath)
	ctx2, cancel := context.WithCancel(ctx)

	// start scheduler goroutine (pass resolved cron expression)
	go runScheduler(ctx2, eff, auditPath, cronExpr)

	logger.Info("retention_scheduler_started", "path", auditPath)
	return cancel, nil
}

// runScheduler wakes periodically and triggers retention runs according
// to the configured cron expression (simple minute/hour matcher).
const defaultLockTTL = 300000 * time.Millisecond

// runScheduler uses gronx to compute the next tick for the configured cron
// expression and sleeps until that time. This yields sharper scheduling and
// supports full cron syntax.
func runScheduler(ctx context.Context, eff config.EffectiveConfigResult, auditPath string, cronExpr string) {
	for {
		select {
		case <-ctx.Done():
			logger.Info("retention_scheduler_stopping", nil)
			return
		default:
		}

		// compute next tick after now (UTC). allowCurrent=false so we get the
		// next future tick.
		now := time.Now().UTC()
		next, err := gronx.NextTickAfter(cronExpr, now, false)
		if err != nil {
			logger.Error("retention_nexttick_failed", "cron", cronExpr, "error", err)
			// fallback sleep then retry
			select {
			case <-time.After(30 * time.Second):
			case <-ctx.Done():
				logger.Info("retention_scheduler_stopping", nil)
				return
			}
			continue
		}

		wait := time.Until(next)
		if wait <= 0 {
			// due now-ish; run immediately
			go func() {
				if err := runOnce(ctx, eff, auditPath); err != nil {
					logger.Error("retention_run_error", "error", err)
				}
			}()
			// small sleep to avoid tight loop
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				logger.Info("retention_scheduler_stopping", nil)
				return
			}
			continue
		}

		// wait until the exact next tick or cancellation
		select {
		case <-time.After(wait):
			go func() {
				if err := runOnce(ctx, eff, auditPath); err != nil {
					logger.Error("retention_run_error", "error", err)
				}
			}()
		case <-ctx.Done():
			logger.Info("retention_scheduler_stopping")
			return
		}
	}
}
