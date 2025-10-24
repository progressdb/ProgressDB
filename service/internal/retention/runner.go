package retention

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"progressdb/pkg/config"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	thread_store "progressdb/pkg/store/threads"
	"progressdb/pkg/timeutil"
)

// runOnce executes a single retention run: acquire lease, scan threads, purge eligible items, write audit.
func runOnce(ctx context.Context, eff config.EffectiveConfigResult, auditPath string) error {
	ret := eff.Config.Retention
	owner := keys.GenMessageID()
	lock := NewFileLease(auditPath)
	acq, err := lock.Acquire(owner, ret.LockTTL.Duration())
	if err != nil {
		logger.Error("retention_lease_acquire_error", "error", err)
		return fmt.Errorf("lease acquire failed: %w", err)
	}
	if !acq {
		logger.Info("retention_lease_not_acquired")
		return nil
	}
	logger.Info("retention_lease_acquired", "owner", owner)
	defer func() {
		if err := lock.Release(owner); err != nil {
			logger.Error("retention_lease_release_error", "error", err)
		} else {
			logger.Info("retention_lease_released", "owner", owner)
		}
	}()

	// create a cancellable run context that will be used to abort the run if
	// the lease cannot be renewed repeatedly
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	// start heartbeat goroutine which will attempt to renew the lease and
	// abort the run if renew fails repeatedly
	hbCtx, hbCancel := context.WithCancel(runCtx)
	go func() {
		t := time.NewTicker(ret.LockTTL.Duration() / 3)
		defer t.Stop()
		defer hbCancel()
		var failCount int
		const maxConsecutiveRenewFails = 3
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-t.C:
				if err := lock.Renew(owner, ret.LockTTL.Duration()); err != nil {
					failCount++
					logger.Error("retention_lease_renew_failed", "error", err, "count", failCount)
					if failCount >= maxConsecutiveRenewFails {
						logger.Error("retention_lease_renew_failed_fatal", "owner", owner)
						// abort the run
						runCancel()
						return
					}
				} else {
					// reset on success
					if failCount != 0 {
						logger.Info("retention_lease_renew_succeeded_after_failures", "owner", owner, "recovered_count", failCount)
					}
					failCount = 0
				}
			}
		}
	}()
	defer hbCancel()

	// open audit writer
	runID := keys.GenMessageID()
	logger.Info("retention_run_start", "run_id", runID, "owner", owner, "dry_run", ret.DryRun)
	// header (emit audit event via dedicated audit logger if present)
	if logger.Audit != nil {
		logger.Audit.Info("retention_audit_header", "run_id", runID, "started_at", timeutil.Now().Format(time.RFC3339), "dry_run", ret.DryRun, "period", ret.Period)
	} else {
		logger.Info("retention_audit_header", "run_id", runID, "started_at", timeutil.Now().Format(time.RFC3339), "dry_run", ret.DryRun, "period", ret.Period)
	}

	// compute cutoff
	pd, perr := parseRetention(ret.Period)
	if perr != nil {
		logger.Error("retention_invalid_period", "period", ret.Period, "error", perr)
		return fmt.Errorf("invalid retention period: %w", perr)
	}
	cutoff := timeutil.Now().Add(-pd)

	threads, err := listAllThreads()
	if err != nil {
		return fmt.Errorf("list threads: %w", err)
	}
	var scanned, purged int
	for _, ts := range threads {
		// if the run context was canceled (for example due to lease renew
		// failures), abort processing promptly
		select {
		case <-runCtx.Done():
			return fmt.Errorf("retention run aborted due to lease renewal failures")
		default:
		}
		var th models.Thread
		if err := json.Unmarshal([]byte(ts), &th); err != nil {
			logger.Error("retention_invalid_thread_json", "error", err)
			continue
		}
		scanned++
		if th.Deleted && time.Unix(0, th.DeletedTS).Before(cutoff) {
			// eligible
			entry := map[string]interface{}{"item_type": "thread", "item_id": th.ID}
			if ret.DryRun {
				entry["status"] = "dry_run"
				if logger.Audit != nil {
					logger.Audit.Info("retention_audit_item", "run_id", runID, "item", entry)
				} else {
					logger.Info("retention_audit_item", "run_id", runID, "item", entry)
				}
				continue
			}
			// attempt purge
			err := thread_store.PurgeThreadPermanently(th.ID)
			if err != nil {
				entry["status"] = "failed"
				entry["error"] = err.Error()
				if logger.Audit != nil {
					logger.Audit.Info("retention_audit_item", "run_id", runID, "item", entry)
				} else {
					logger.Info("retention_audit_item", "run_id", runID, "item", entry)
				}
				logger.Error("retention_purge_failed", "thread", th.ID, "error", err)
				continue
			}
			entry["status"] = "success"
			entry["purged_at"] = timeutil.Now().Format(time.RFC3339)
			if logger.Audit != nil {
				logger.Audit.Info("retention_audit_item", "run_id", runID, "item", entry)
			} else {
				logger.Info("retention_audit_item", "run_id", runID, "item", entry)
			}
			purged++
			logger.Info("retention_item_purged", "type", "thread", "id", th.ID)
		}
	}

	if logger.Audit != nil {
		logger.Audit.Info("retention_audit_footer", "run_id", runID, "scanned", scanned, "purged", purged)
	} else {
		logger.Info("retention_audit_footer", "run_id", runID, "scanned", scanned, "purged", purged)
	}
	logger.Info("retention_run_complete", "scanned", scanned, "purged", purged)
	return nil
}

// listAllThreads lists all threads in the system
func listAllThreads() ([]string, error) {
	// Use storedb to list all thread metadata keys
	keys, _, _, err := storedb.ListKeys("t:", 10000, "")
	if err != nil {
		return nil, err
	}

	var threadKeys []string
	for _, key := range keys {
		if strings.HasSuffix(key, ":meta") {
			threadKeys = append(threadKeys, key)
		}
	}

	// Fetch thread data for each key
	var threads []string
	for _, key := range threadKeys {
		val, err := storedb.GetKey(key)
		if err != nil {
			continue
		}
		threads = append(threads, val)
	}

	return threads, nil
}

func parseRetention(s string) (time.Duration, error) {
	// supports 30d, 24h, etc. default to 30d when empty
	if s == "" {
		return 30 * 24 * time.Hour, nil
	}
	if s[len(s)-1] == 'd' {
		n := 0
		if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
			return 0, fmt.Errorf("invalid days retention: %w", err)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return d, nil
}
