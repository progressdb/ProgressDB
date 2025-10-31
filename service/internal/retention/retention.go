package retention

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/adhocore/gronx"

	"progressdb/pkg/config"
	"progressdb/pkg/models"
	"progressdb/pkg/state"
	"progressdb/pkg/state/logger"
	storedb "progressdb/pkg/store/db/storedb"
	thread_store "progressdb/pkg/store/features/threads"
	"progressdb/pkg/store/pagination"
	"progressdb/pkg/timeutil"
)

// RetentionManager manages simplified retention for single-node deployments
type RetentionManager struct {
	mutex   sync.Mutex
	config  *config.RetentionConfig
	ctx     context.Context
	cancel  context.CancelFunc
	pidFile string
}

var globalManager *RetentionManager

// Start initializes and starts the retention scheduler
func Start(ctx context.Context) (context.CancelFunc, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("no config available")
	}

	ret := cfg.Retention
	if !ret.Enabled {
		logger.Info("retention_disabled")
		return func() {}, nil
	}

	if ret.Paused {
		logger.Info("retention_paused")
		return func() {}, nil
	}

	// ensure retention directory exists
	if state.PathsVar.Retention == "" {
		return nil, fmt.Errorf("state paths not initialized")
	}
	if err := os.MkdirAll(state.PathsVar.Retention, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create retention directory: %w", err)
	}

	// initialize global manager
	ctx2, cancel := context.WithCancel(ctx)
	globalManager = &RetentionManager{
		config:  &ret,
		ctx:     ctx2,
		cancel:  cancel,
		pidFile: filepath.Join(state.PathsVar.Retention, "retention.pid"),
	}

	logger.Info("retention_enabled", "cron", ret.Cron, "period", ret.Period, "pid_file", globalManager.pidFile)

	// start scheduler goroutine
	go globalManager.runScheduler()

	return cancel, nil
}

// RunImmediate executes retention immediately using current config
func RunImmediate() error {
	if globalManager == nil {
		return fmt.Errorf("retention manager not initialized")
	}
	return globalManager.runOnce()
}

// runScheduler handles cron-based scheduling
func (rm *RetentionManager) runScheduler() {
	for {
		select {
		case <-rm.ctx.Done():
			logger.Info("retention_scheduler_stopping")
			return
		default:
		}

		// calculate next tick after now (UTC)
		now := timeutil.Now()
		next, err := gronx.NextTickAfter(rm.config.Cron, now, false)
		if err != nil {
			logger.Error("retention_nexttick_failed", "cron", rm.config.Cron, "error", err)
			select {
			case <-time.After(30 * time.Second):
			case <-rm.ctx.Done():
				return
			}
			continue
		}

		wait := time.Until(next)
		if wait <= 0 {
			// time is due, run immediately
			go func() {
				if err := rm.runOnce(); err != nil {
					logger.Error("retention_run_error", "error", err)
				}
			}()
			// avoid tight loop
			select {
			case <-time.After(time.Second):
			case <-rm.ctx.Done():
				return
			}
			continue
		}

		// wait until next tick or cancellation
		select {
		case <-time.After(wait):
			go func() {
				if err := rm.runOnce(); err != nil {
					logger.Error("retention_run_error", "error", err)
				}
			}()
		case <-rm.ctx.Done():
			return
		}
	}
}

// acquireLock obtains exclusive lock using PID file
func (rm *RetentionManager) acquireLock() bool {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	// check if existing process is running
	if data, err := os.ReadFile(rm.pidFile); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			if rm.isProcessRunning(pid) {
				logger.Info("retention_already_running", "pid", pid)
				return false
			}
		}
		// stale PID file, remove it
		os.Remove(rm.pidFile)
	}

	// write current PID
	pid := os.Getpid()
	if err := os.WriteFile(rm.pidFile, []byte(strconv.Itoa(pid)), 0o600); err != nil {
		logger.Error("retention_pid_write_failed", "error", err)
		return false
	}

	logger.Info("retention_lock_acquired", "pid", pid)
	return true
}

// releaseLock removes the PID file
func (rm *RetentionManager) releaseLock() {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if err := os.Remove(rm.pidFile); err != nil && !os.IsNotExist(err) {
		logger.Error("retention_pid_remove_failed", "error", err)
	} else {
		logger.Info("retention_lock_released")
	}
}

// isProcessRunning checks if a process with given PID exists
func (rm *RetentionManager) isProcessRunning(pid int) bool {
	// On Unix systems, we can check if process exists by sending signal 0
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Try to send signal 0 (doesn't actually kill process)
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// runOnce executes a single retention run
func (rm *RetentionManager) runOnce() error {
	if !rm.acquireLock() {
		return nil
	}
	defer rm.releaseLock()

	// generate run ID for tracking
	runID := fmt.Sprintf("run-%d", timeutil.Now().UnixNano())
	logger.Info("retention_run_start", "run_id", runID, "dry_run", rm.config.DryRun)

	// parse retention period
	retentionPeriod, err := rm.parseRetentionPeriod(rm.config.Period)
	if err != nil {
		return fmt.Errorf("invalid retention period: %w", err)
	}
	cutoff := timeutil.Now().Add(-retentionPeriod)

	// scan for deleted items
	deletedItems, err := rm.scanDeletedItems(cutoff)
	if err != nil {
		return fmt.Errorf("scan deleted items: %w", err)
	}

	logger.Info("retention_scan_complete", "run_id", runID, "eligible_items", len(deletedItems))

	// purge items if not dry run
	var purged int
	if !rm.config.DryRun {
		purged, err = rm.purgeItems(deletedItems)
		if err != nil {
			return fmt.Errorf("purge items: %w", err)
		}
	} else {
		// log what would be purged in dry run mode
		for _, item := range deletedItems {
			logger.Info("retention_dry_run", "run_id", runID, "would_purge", item)
		}
		purged = len(deletedItems)
	}

	logger.Info("retention_run_complete", "run_id", runID, "scanned", len(deletedItems), "purged", purged)
	return nil
}

// scanDeletedItems finds items marked for deletion that are older than cutoff
func (rm *RetentionManager) scanDeletedItems(cutoff time.Time) ([]string, error) {
	var eligibleItems []string

	// scan soft delete markers with prefix "del:"
	prefix := "del:"
	keys, _, err := storedb.ListKeysWithPrefixPaginated(prefix, &pagination.PaginationRequest{Limit: 10000, Cursor: ""})
	if err != nil {
		return nil, fmt.Errorf("list soft delete markers: %w", err)
	}

	for _, deleteMarker := range keys {
		// extract original key from delete marker
		originalKey := strings.TrimPrefix(deleteMarker, prefix)
		if originalKey == "" {
			continue
		}

		// check if this is a thread delete marker
		if strings.HasPrefix(originalKey, "t:") {
			// get thread metadata to check deletion timestamp
			threadData, err := storedb.GetKey(originalKey)
			if err != nil {
				logger.Debug("retention_thread_not_found", "key", originalKey)
				continue
			}

			var thread models.Thread
			if err := json.Unmarshal([]byte(threadData), &thread); err != nil {
				logger.Error("retention_invalid_thread_json", "key", originalKey, "error", err)
				continue
			}

			// check if thread is deleted and older than cutoff
			if thread.Deleted && thread.DeletedTS > 0 {
				deletedTime := time.Unix(0, thread.DeletedTS)
				if deletedTime.Before(cutoff) {
					eligibleItems = append(eligibleItems, originalKey)
				}
			}
		}
	}

	return eligibleItems, nil
}

// purgeItems permanently deletes the specified threads
func (rm *RetentionManager) purgeItems(threadKeys []string) (int, error) {
	var purged int
	var errors []string

	for _, threadKey := range threadKeys {
		if err := thread_store.PurgeThreadPermanently(threadKey); err != nil {
			logger.Error("retention_purge_failed", "thread", threadKey, "error", err)
			errors = append(errors, fmt.Sprintf("%s: %v", threadKey, err))
		} else {
			purged++
			logger.Info("retention_item_purged", "type", "thread", "key", threadKey)
		}
	}

	if len(errors) > 0 {
		logger.Error("retention_purge_errors", "errors", strings.Join(errors, "; "))
	}

	return purged, nil
}

// parseRetentionPeriod converts period string to duration
func (rm *RetentionManager) parseRetentionPeriod(period string) (time.Duration, error) {
	if period == "" {
		return 30 * 24 * time.Hour, nil // default 30 days
	}

	// support "d" suffix for days
	if strings.HasSuffix(period, "d") {
		days := 0
		if _, err := fmt.Sscanf(period, "%d", &days); err != nil {
			return 0, fmt.Errorf("invalid days format: %w", err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	// try standard duration parsing
	duration, err := time.ParseDuration(period)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format: %w", err)
	}

	return duration, nil
}
