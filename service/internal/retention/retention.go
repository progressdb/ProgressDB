package retention

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/adhocore/gronx"

	"progressdb/pkg/config"
	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/iterator/admin/ki"
	"progressdb/pkg/store/keys"
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
		logger.Info("[RETENTION] retention_disabled")
		return func() {}, nil
	}

	ctx2, cancel := context.WithCancel(ctx)
	rm := &RetentionManager{
		cfg:     &cfg.Retention,
		ctx:     ctx2,
		cancel:  cancel,
		running: false,
	}

	managerMutex.Lock()
	globalManager = rm
	managerMutex.Unlock()

	logger.Info("[RETENTION] retention_enabled", "cron", rm.cfg.Cron)
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
		now := timeutil.Now()
		next, err := gronx.NextTickAfter(rm.cfg.Cron, now, false)
		if err != nil {
			logger.Error("[RETENTION] retention_nexttick_failed", "cron", rm.cfg.Cron, "error", err)
			select {
			case <-time.After(30 * time.Second):
			case <-rm.ctx.Done():
				return
			}
			continue
		}

		wait := time.Until(next)
		if wait <= 0 {
			rm.runJob()
			select {
			case <-time.After(time.Second):
			case <-rm.ctx.Done():
				return
			}
			continue
		}

		select {
		case <-time.After(wait):
			rm.runJob()
		case <-rm.ctx.Done():
			return
		}
	}
}

func (rm *RetentionManager) runJob() {
	rm.mutex.Lock()
	if rm.running {
		rm.mutex.Unlock()
		return
	}
	rm.running = true
	rm.mutex.Unlock()

	defer func() {
		rm.mutex.Lock()
		rm.running = false
		rm.mutex.Unlock()
	}()

	if err := rm.runPurge(); err != nil {
		logger.Error("[RETENTION] retention_run_error", "error", err)
	}
}

func (rm *RetentionManager) runPurge() error {
	runID := fmt.Sprintf("run-%d", timeutil.Now().UnixNano())
	logger.Info("[RETENTION] retention_run_start", "run_id", runID)

	keyIter := ki.NewKeyIterator(indexdb.Client)
	deleteMarkers, _, err := keyIter.ExecuteKeyQuery(keys.GenSoftDeletePrefix(), pagination.PaginationRequest{Limit: 10000})
	if err != nil {
		return fmt.Errorf("scan soft delete markers: %w", err)
	}

	var purged int
	for _, deleteMarkerKey := range deleteMarkers {
		deleteMarker, err := keys.ParseSoftDeleteMarker(deleteMarkerKey)
		if err != nil {
			logger.Error("[RETENTION] failed_to_parse_delete_marker", "marker", deleteMarkerKey, "error", err)
			continue
		}

		originalKey := deleteMarker.OriginalKey
		if originalKey == "" {
			continue
		}

		parsedOriginalKey, err := keys.ParseKey(originalKey)
		if err != nil || parsedOriginalKey.Type != keys.KeyTypeThread {
			continue
		}

		// parsed keys = thread keys
		data, err := storedb.GetKey(originalKey)
		if err != nil {
			continue
		}

		var thread models.Thread
		if err := json.Unmarshal([]byte(data), &thread); err != nil {
			logger.Error("[RETENTION] retention_invalid_thread_json", "key", originalKey, "error", err)
			continue
		}

		// second check
		if thread.Deleted {
			deletedTime := time.Unix(0, thread.UpdatedTS)
			age := time.Since(deletedTime)
			if age > rm.cfg.TTTL {
				// purge thread & its associated resources
				if err := rm.purgeThreadCompletely(originalKey); err != nil {
					logger.Error("[RETENTION] purge_failed", "key", originalKey, "error", err)
				} else {
					purged++
				}
			}
		}
	}

	logger.Info("[RETENTION] retention_run_done", "run_id", runID, "scanned", len(deleteMarkers), "purged", purged)
	return nil
}

func (rm *RetentionManager) purgeThreadCompletely(threadKey string) error {
	parsed, err := keys.ParseKey(threadKey)
	if err != nil {
		return fmt.Errorf("parse thread key: %w", err)
	}
	if parsed.Type != keys.KeyTypeThread {
		return fmt.Errorf("expected thread key, got %s", parsed.Type)
	}

	threadUserPrefix, err := keys.GenThreadUserRelPrefix(threadKey)
	if err != nil {
		return fmt.Errorf("generate thread user prefix: %w", err)
	}

	userIDs, relErr := rm.getUsersFromThreadRelationships(threadUserPrefix)
	if relErr != nil {
		logger.Error("[RETENTION] failed_to_get_users_from_thread", "prefix", threadUserPrefix, "error", relErr)
	}

	for _, userID := range userIDs {
		fullUserThreadKey, err := keys.GenUserThreadRelPrefix(userID)
		if err != nil {
			logger.Error("[RETENTION] failed_to_generate_user_thread_prefix", "user_id", userID, "error", err)
			continue
		}
		fullUserThreadKey += parsed.ThreadTS
		if err := indexdb.DeleteKey(fullUserThreadKey); err != nil {
			logger.Error("[RETENTION] failed_to_delete_user_thread_rel", "key", fullUserThreadKey, "error", err)
		}
	}

	if err := rm.deleteByPrefixFromIndexDB(threadUserPrefix); err != nil {
		logger.Error("[RETENTION] failed_to_delete_thread_user_rels", "prefix", threadUserPrefix, "error", err)
	}

	if err := storedb.DeleteKey(threadKey); err != nil {
		logger.Error("[RETENTION] failed_to_delete_thread_metadata", "key", threadKey, "error", err)
	}

	threadIndexPrefix, err := keys.GenThreadIndexPrefix(threadKey)
	if err != nil {
		logger.Error("[RETENTION] failed_to_generate_thread_index_prefix", "thread_key", threadKey, "error", err)
		return err
	}
	if err := rm.deleteByPrefixFromIndexDB(threadIndexPrefix); err != nil {
		logger.Error("[RETENTION] failed_to_delete_thread_indexes", "prefix", threadIndexPrefix, "error", err)
	}

	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		logger.Error("[RETENTION] failed_to_generate_message_prefix", "thread_key", threadKey, "error", err)
		return err
	}
	if err := rm.deleteByPrefixFromStoreDB(messagePrefix); err != nil {
		logger.Error("[RETENTION] failed_to_delete_thread_messages", "prefix", messagePrefix, "error", err)
	}

	deleteMarker := keys.GenSoftDeleteMarkerKey(threadKey)
	if err := indexdb.DeleteKey(deleteMarker); err != nil {
		logger.Error("[RETENTION] failed_to_delete_soft_delete_marker", "marker", deleteMarker, "error", err)
	}

	return nil
}

func (rm *RetentionManager) getUsersFromThreadRelationships(threadUserPrefix string) ([]string, error) {
	keyIter := ki.NewKeyIterator(indexdb.Client)
	relKeys, _, err := keyIter.ExecuteKeyQuery(threadUserPrefix, pagination.PaginationRequest{Limit: 10000})
	if err != nil {
		return nil, fmt.Errorf("scan thread-user relationships: %w", err)
	}

	var userIDs []string
	for _, key := range relKeys {
		parsed, err := keys.ParseKey(key)
		if err != nil {
			continue
		}
		if parsed.Type == keys.KeyTypeThreadHasUser && parsed.UserID != "" {
			userIDs = append(userIDs, parsed.UserID)
		}
	}

	return userIDs, nil
}

func (rm *RetentionManager) deleteByPrefixFromIndexDB(prefix string) error {
	keyIter := ki.NewKeyIterator(indexdb.Client)
	keys, _, err := keyIter.ExecuteKeyQuery(prefix, pagination.PaginationRequest{Limit: 10000})
	if err != nil {
		return fmt.Errorf("scan keys with prefix %s: %w", prefix, err)
	}

	for _, key := range keys {
		if err := indexdb.DeleteKey(key); err != nil {
			logger.Error("[RETENTION] failed_to_delete_key", "key", key, "error", err)
			continue
		}
	}

	return nil
}

func (rm *RetentionManager) deleteByPrefixFromStoreDB(prefix string) error {
	keyIter := ki.NewKeyIterator(storedb.Client)
	keys, _, err := keyIter.ExecuteKeyQuery(prefix, pagination.PaginationRequest{Limit: 10000})
	if err != nil {
		return fmt.Errorf("scan keys with prefix %s: %w", prefix, err)
	}

	for _, key := range keys {
		if err := storedb.DeleteKey(key); err != nil {
			logger.Error("[RETENTION] failed_to_delete_key", "key", key, "error", err)
			continue
		}
	}

	return nil
}
