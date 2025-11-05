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
		now := timeutil.Now()
		next, err := gronx.NextTickAfter(rm.cfg.Cron, now, false)
		if err != nil {
			logger.Error("retention_nexttick_failed", "cron", rm.cfg.Cron, "error", err)
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
		logger.Error("retention_run_error", "error", err)
	}
}

func (rm *RetentionManager) runPurge() error {
	runID := fmt.Sprintf("run-%d", timeutil.Now().UnixNano())
	logger.Info("retention_run_start", "run_id", runID)

	keyIter := ki.NewKeyIterator(indexdb.Client)
	deleteMarkers, _, err := keyIter.ExecuteKeyQuery(keys.GenSoftDeletePrefix(), pagination.PaginationRequest{Limit: 10000})
	if err != nil {
		return fmt.Errorf("scan soft delete markers: %w", err)
	}
	logger.Info("retention_scan_keys", "count", len(deleteMarkers))

	var purged int
	for _, deleteMarkerKey := range deleteMarkers {
		logger.Info("retention_key", "raw_delete_marker", deleteMarkerKey)

		deleteMarker, err := keys.ParseSoftDeleteMarker(deleteMarkerKey)
		if err != nil {
			logger.Error("failed_to_parse_delete_marker", "marker", deleteMarkerKey, "error", err)
			continue
		}

		originalKey := deleteMarker.OriginalKey
		logger.Info("retention_key", "original_key", originalKey)
		if originalKey == "" {
			continue
		}

		parsedOriginalKey, err := keys.ParseKey(originalKey)
		if err != nil || parsedOriginalKey.Type != keys.KeyTypeThread {
			logger.Debug("skipping_non_thread_delete_marker", "original_key", originalKey, "type", parsedOriginalKey.Type, "error", err)
			continue
		}

		data, err := storedb.GetKey(originalKey)
		if err != nil {
			logger.Info("retention_thread_not_found", "key", originalKey, "error", err)
			continue
		}

		var thread models.Thread
		if err := json.Unmarshal([]byte(data), &thread); err != nil {
			logger.Error("retention_invalid_thread_json", "key", originalKey, "error", err)
			continue
		}

		if thread.Deleted {
			deletedTime := time.Unix(0, thread.UpdatedTS)
			age := time.Since(deletedTime)
			logger.Info("retention_check", "key", originalKey, "deleted", thread.Deleted, "deleted_time", deletedTime, "age", age, "tttl", rm.cfg.TTTL, "should_purge", age > rm.cfg.TTTL)
			if age > rm.cfg.TTTL {
				if err := rm.purgeThreadCompletely(originalKey); err != nil {
					logger.Error("purge_failed", "key", originalKey, "error", err)
				} else {
					purged++
					logger.Info("purged", "key", originalKey, "deleted_age", age)
				}
			} else {
				logger.Info("thread_not_old_enough_for_purge", "key", originalKey, "deleted_age", age, "tttl", rm.cfg.TTTL)
			}
		}
	}

	logger.Info("retention_run_done", "run_id", runID, "scanned", len(deleteMarkers), "purged", purged)
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

	// Get thread-user relationships to find all users in this thread
	// Use manual construction since GenThreadUserRelPrefix expects full thread key format
	threadUserPrefix := fmt.Sprintf("rel:t:%s:u:", parsed.ThreadTS)
	logger.Info("retention_debug", "action", "generated_thread_user_prefix", "prefix", threadUserPrefix, "thread_ts", parsed.ThreadTS, "thread_key", parsed.ThreadKey)

	userIDs, relErr := rm.getUsersFromThreadRelationships(threadUserPrefix)
	if relErr != nil {
		logger.Error("failed_to_get_users_from_thread", "prefix", threadUserPrefix, "error", relErr)
	}
	logger.Info("retention_debug", "action", "found_user_ids", "user_ids", userIDs, "count", len(userIDs))

	// Delete user-thread relationships for each user found (from indexdb)
	for _, userID := range userIDs {
		// Build full key manually: rel:u:{userID}:t:{threadTS}
		fullUserThreadKey := fmt.Sprintf("rel:u:%s:t:%s", userID, parsed.ThreadTS)
		logger.Info("retention_debug", "action", "deleting_user_thread_rel", "user_id", userID, "full_key", fullUserThreadKey)
		if err := indexdb.DeleteKey(fullUserThreadKey); err != nil {
			logger.Error("failed_to_delete_user_thread_rel", "key", fullUserThreadKey, "error", err)
		} else {
			logger.Info("retention_debug", "action", "deleted_user_thread_rel", "key", fullUserThreadKey)
		}
	}

	// Delete thread-user relationships (from indexdb)
	logger.Info("retention_debug", "action", "deleting_thread_user_rels", "prefix", threadUserPrefix)
	if err := rm.deleteByPrefixFromIndexDB(threadUserPrefix); err != nil {
		logger.Error("failed_to_delete_thread_user_rels", "prefix", threadUserPrefix, "error", err)
	} else {
		logger.Info("retention_debug", "action", "deleted_thread_user_rels", "prefix", threadUserPrefix)
	}

	// Delete thread metadata (from storedb)
	if err := storedb.DeleteKey(threadKey); err != nil {
		logger.Error("failed_to_delete_thread_metadata", "key", threadKey, "error", err)
	}

	// Delete thread indexes (from indexdb)
	threadIndexPrefix := fmt.Sprintf("idx:t:%s:", parsed.ThreadTS)
	if err := rm.deleteByPrefixFromIndexDB(threadIndexPrefix); err != nil {
		logger.Error("failed_to_delete_thread_indexes", "prefix", threadIndexPrefix, "error", err)
	}

	// Delete all messages in thread (from storedb)
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		logger.Error("failed_to_generate_message_prefix", "thread_key", threadKey, "error", err)
		return err
	}
	if err := rm.deleteByPrefixFromStoreDB(messagePrefix); err != nil {
		logger.Error("failed_to_delete_thread_messages", "prefix", messagePrefix, "error", err)
	}

	// Delete soft delete marker (from indexdb)
	deleteMarker := keys.GenSoftDeleteMarkerKey(threadKey)
	if err := indexdb.DeleteKey(deleteMarker); err != nil {
		logger.Error("failed_to_delete_soft_delete_marker", "marker", deleteMarker, "error", err)
	}

	return nil
}

func (rm *RetentionManager) getUsersFromThreadRelationships(threadUserPrefix string) ([]string, error) {
	logger.Info("retention_debug", "action", "scanning_thread_user_rels", "prefix", threadUserPrefix)
	keyIter := ki.NewKeyIterator(indexdb.Client)
	relKeys, _, err := keyIter.ExecuteKeyQuery(threadUserPrefix, pagination.PaginationRequest{Limit: 10000})
	if err != nil {
		return nil, fmt.Errorf("scan thread-user relationships: %w", err)
	}
	logger.Info("retention_debug", "action", "found_thread_user_rel_keys", "keys", relKeys, "count", len(relKeys))

	var userIDs []string
	for _, key := range relKeys {
		logger.Info("retention_debug", "action", "parsing_thread_user_rel", "key", key)
		parsed, err := keys.ParseKey(key)
		if err != nil {
			logger.Error("failed_to_parse_thread_user_rel", "key", key, "error", err)
			continue
		}
		if parsed.Type == keys.KeyTypeThreadHasUser && parsed.UserID != "" {
			userIDs = append(userIDs, parsed.UserID)
			logger.Info("retention_debug", "action", "extracted_user_id", "user_id", parsed.UserID, "from_key", key)
		}
	}

	return userIDs, nil
}

func (rm *RetentionManager) deleteByPrefixFromIndexDB(prefix string) error {
	logger.Info("retention_debug", "action", "scanning_indexdb_keys", "prefix", prefix)
	keyIter := ki.NewKeyIterator(indexdb.Client)
	keys, _, err := keyIter.ExecuteKeyQuery(prefix, pagination.PaginationRequest{Limit: 10000})
	if err != nil {
		return fmt.Errorf("scan keys with prefix %s: %w", prefix, err)
	}
	logger.Info("retention_debug", "action", "found_indexdb_keys", "keys", keys, "count", len(keys))

	for _, key := range keys {
		logger.Info("retention_debug", "action", "deleting_indexdb_key", "key", key)
		if err := indexdb.DeleteKey(key); err != nil {
			logger.Error("failed_to_delete_key", "key", key, "error", err)
			continue
		} else {
			logger.Info("retention_debug", "action", "deleted_indexdb_key", "key", key)
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
			logger.Error("failed_to_delete_key", "key", key, "error", err)
			continue
		}
	}

	return nil
}
