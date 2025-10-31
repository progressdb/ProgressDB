package retention

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/adhocore/gronx"

	"progressdb/pkg/config"
	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"
	thread_store "progressdb/pkg/store/features/threads"
	"progressdb/pkg/store/pagination"
	"progressdb/pkg/timeutil"
)

var (
	globalManager *RetentionManager
	managerMutex  sync.Mutex
)

type RetentionManager struct {
	cfg     *config.RetentionConfig
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	mutex   sync.Mutex
}

func Start(ctx context.Context) (context.CancelFunc, error) {
	cfg := config.GetConfig()
	if cfg == nil || !cfg.Retention.Enabled {
		logger.Info("retention_disabled")
		return func() {}, nil
	}

	ctx2, cancel := context.WithCancel(ctx)
	rm := &RetentionManager{
		cfg:     &cfg.Retention,
		ctx:     ctx2,
		cancel:  cancel,
		running: false,
	}

	// Store global manager for RunImmediate()
	managerMutex.Lock()
	globalManager = rm
	managerMutex.Unlock()

	logger.Info("retention_enabled", "cron", rm.cfg.Cron)
	go rm.scheduleLoop()
	return cancel, nil
}

func RunImmediate() error {
	managerMutex.Lock()
	rm := globalManager
	managerMutex.Unlock()

	if rm == nil {
		return fmt.Errorf("retention manager not initialized - call Start() first")
	}

	return rm.runPurge()
}

func (rm *RetentionManager) scheduleLoop() {
	for {
		// Calculate next run time using gronx
		now := timeutil.Now()
		next, err := gronx.NextTickAfter(rm.cfg.Cron, now, false)
		if err != nil {
			logger.Error("retention_nexttick_failed", "cron", rm.cfg.Cron, "error", err)
			// Fallback: retry after 30 seconds
			select {
			case <-time.After(30 * time.Second):
			case <-rm.ctx.Done():
				return
			}
			continue
		}

		wait := time.Until(next)
		if wait <= 0 {
			// Time is due, run immediately
			rm.executeRetention()
			// Avoid tight loop - wait a bit before recalculating
			select {
			case <-time.After(time.Second):
			case <-rm.ctx.Done():
				return
			}
			continue
		}

		// Wait until next run time or cancellation
		select {
		case <-time.After(wait):
			rm.executeRetention()
		case <-rm.ctx.Done():
			return
		}
	}
}

func (rm *RetentionManager) executeRetention() {
	rm.mutex.Lock()
	if rm.running {
		rm.mutex.Unlock()
		return // Already running
	}
	rm.running = true
	rm.mutex.Unlock()

	defer func() {
		rm.mutex.Lock()
		rm.running = false
		rm.mutex.Unlock()
	}()

	if err := rm.runPurge(); err != nil {
		logger.Error("retention_run_error", "error", err)
	}
}

func (rm *RetentionManager) runPurge() error {
	runID := fmt.Sprintf("run-%d", timeutil.Now().UnixNano())
	logger.Info("retention_run_start", "run_id", runID)

	// Scan for deleted items
	keys, _, err := indexdb.ListKeysWithPrefixPaginated("del:", &pagination.PaginationRequest{Limit: 10000})
	if err != nil {
		return fmt.Errorf("scan soft delete markers: %w", err)
	}

	var purged int
	for _, deleteMarker := range keys {
		// Extract original key from delete marker
		originalKey := strings.TrimPrefix(deleteMarker, "del:")
		if originalKey == "" {
			continue
		}

		// Check if this is a thread delete marker
		if strings.HasPrefix(originalKey, "t:") {
			data, err := storedb.GetKey(originalKey)
			if err != nil {
				logger.Debug("retention_thread_not_found", "key", originalKey)
				continue
			}

			var thread models.Thread
			if err := json.Unmarshal([]byte(data), &thread); err != nil {
				logger.Error("retention_invalid_thread_json", "key", originalKey, "error", err)
				continue
			}

			// Check if thread is deleted
			if thread.Deleted {
				if err := thread_store.PurgeThreadPermanently(originalKey); err != nil {
					logger.Error("purge_failed", "key", originalKey, "error", err)
				} else {
					purged++
					logger.Info("purged", "key", originalKey)
				}
			}
		}
	}

	logger.Info("retention_run_done", "run_id", runID, "scanned", len(keys), "purged", purged)
	return nil
}
