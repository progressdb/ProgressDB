package threads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
)

func PurgeThreadPermanently(threadKey string) error {
	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}

	if threadKey == "" {
		return fmt.Errorf("threadKey cannot be empty")
	}

	const deleteBatchSize = 1000
	var deletedKeys int
	var mainBatch [][]byte
	var versionKeys [][]byte

	deleteMainBatch := func(keys [][]byte) {
		for _, k := range keys {
			if err := storedb.Client.Delete(k, storedb.WriteOpt(true)); err != nil {
				logger.Error("purge_delete_failed", "key", string(k), "error", err)
			} else {
				deletedKeys++
			}
		}
	}

	deleteVersionBatch := func(keys [][]byte) {
		for _, k := range keys {
			if err := storedb.Client.Delete(k, storedb.WriteOpt(true)); err != nil {
				logger.Error("purge_version_delete_failed", "key", string(k), "error", err)
			} else {
				deletedKeys++
			}
		}
	}

	threadKey = keys.GenThreadKey(threadKey)
	if err := storedb.Client.Delete([]byte(threadKey), storedb.WriteOpt(true)); err != nil {
		logger.Error("delete_thread_meta_failed", "thread", threadKey, "error", err)
		return fmt.Errorf("failed to delete thread metadata: %w", err)
	} else {
		deletedKeys++
	}

	threadPrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return fmt.Errorf("failed to generate thread prefix: %w", err)
	}

	lowerBound := []byte(threadPrefix)
	upperBound := calculateUpperBound(threadPrefix)

	iter, err := storedb.Client.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := append([]byte(nil), iter.Key()...)
		mainBatch = append(mainBatch, key)

		if bytes.Contains(key, []byte(":m:")) {
			value := append([]byte(nil), iter.Value()...)
			var m models.Message
			if err := json.Unmarshal(value, &m); err == nil && m.Key != "" {
				versionPrefix, err := keys.GenAllMessageVersionsPrefix(m.Key)
				if err != nil {
					logger.Error("failed_to_generate_version_prefix", "error", err)
					continue
				}
				vIter, err := storedb.Client.NewIter(&pebble.IterOptions{
					LowerBound: []byte(versionPrefix),
					UpperBound: calculateUpperBound(versionPrefix),
				})
				if err == nil {
					for vIter.First(); vIter.Valid(); vIter.Next() {
						vKey := append([]byte(nil), vIter.Key()...)
						versionKeys = append(versionKeys, vKey)
						if len(versionKeys) >= deleteBatchSize {
							deleteVersionBatch(versionKeys)
							versionKeys = versionKeys[:0]
						}
					}
					vIter.Close()
				}
			}
		}

		if len(mainBatch) >= deleteBatchSize {
			deleteMainBatch(mainBatch)
			mainBatch = mainBatch[:0]
		}
	}

	if len(mainBatch) > 0 {
		deleteMainBatch(mainBatch)
	}
	if len(versionKeys) > 0 {
		deleteVersionBatch(versionKeys)
	}

	if err := indexdb.DeleteThreadMessageIndexes(threadKey); err != nil {
		logger.Error("delete_thread_message_indexes_failed", "thread", threadKey, "error", err)
	}

	if err := indexdb.UnmarkSoftDeleted(threadKey); err != nil {
		logger.Error("unmark_thread_soft_deleted_purge_failed", "thread", threadKey, "error", err)
	}

	if err := cleanupThreadRelationships(threadKey); err != nil {
		logger.Error("cleanup_thread_relationships_failed", "thread", threadKey, "error", err)
	}

	logger.Info("purge_thread_completed", "thread", threadKey, "deleted_keys", deletedKeys)
	return nil
}

func cleanupThreadRelationships(threadKey string) error {
	ownershipPrefix := keys.UserThreadsRelPrefix
	var ownershipKeys []string
	cursor := ""
	for {
		keys, resp, err := indexdb.ListKeysWithPrefixPaginated(ownershipPrefix, &pagination.PaginationRequest{Limit: 100, Cursor: cursor})
		if err != nil {
			return fmt.Errorf("list ownership keys: %w", err)
		}
		ownershipKeys = append(ownershipKeys, keys...)
		if !resp.HasMore {
			break
		}
		cursor = resp.NextCursor
	}

	for _, key := range ownershipKeys {
		if strings.Contains(key, fmt.Sprintf(":t:%s", threadKey)) {
			if err := indexdb.DeleteKey(key); err != nil {
				logger.Error("delete_ownership_relationship_failed", "key", key, "error", err)
			}
		}
	}

	participationPrefix, err := keys.GenThreadUserRelPrefix(threadKey)
	if err != nil {
		return fmt.Errorf("failed to generate participation prefix: %w", err)
	}
	var participationKeys []string
	cursor = ""
	for {
		keys, resp, err := indexdb.ListKeysWithPrefixPaginated(participationPrefix, &pagination.PaginationRequest{Limit: 100, Cursor: cursor})
		if err != nil {
			return fmt.Errorf("list participation keys: %w", err)
		}
		participationKeys = append(participationKeys, keys...)
		if !resp.HasMore {
			break
		}
		cursor = resp.NextCursor
	}

	for _, key := range participationKeys {
		if err := indexdb.DeleteKey(key); err != nil {
			logger.Error("delete_participation_relationship_failed", "key", key, "error", err)
		}
	}

	return nil
}

func calculateUpperBound(prefix string) []byte {
	prefixBytes := []byte(prefix)
	upper := make([]byte, len(prefixBytes))
	copy(upper, prefixBytes)

	for i := len(upper) - 1; i >= 0; i-- {
		if upper[i] < 0xFF {
			upper[i]++
			return upper
		}
		upper[i] = 0
	}

	return append(prefixBytes, 0xFF)
}
