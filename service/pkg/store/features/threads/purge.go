package threads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
)

func PurgeThreadPermanently(threadID string) error {
	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}

	if threadID == "" {
		return fmt.Errorf("threadID cannot be empty")
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

	threadKey := keys.GenThreadKey(threadID)
	if err := storedb.Client.Delete([]byte(threadKey), storedb.WriteOpt(true)); err != nil {
		logger.Error("delete_thread_meta_failed", "thread", threadID, "error", err)
		return fmt.Errorf("failed to delete thread metadata: %w", err)
	} else {
		deletedKeys++
	}

	threadPrefix := keys.GenAllThreadMessagesPrefix(threadID)

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
				versionPrefix := keys.GenAllMessageVersionsPrefix(m.Key)
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

	if err := index.DeleteThreadMessageIndexes(threadID); err != nil {
		logger.Error("delete_thread_message_indexes_failed", "thread", threadID, "error", err)
	}

	if err := index.UnmarkSoftDeleted(threadID); err != nil {
		logger.Error("unmark_thread_soft_deleted_purge_failed", "thread", threadID, "error", err)
	}

	if err := cleanupThreadRelationships(threadID); err != nil {
		logger.Error("cleanup_thread_relationships_failed", "thread", threadID, "error", err)
	}

	logger.Info("purge_thread_completed", "thread", threadID, "deleted_keys", deletedKeys)
	return nil
}

func cleanupThreadRelationships(threadID string) error {
	ownershipPrefix := fmt.Sprintf("rel:u:")
	var ownershipKeys []string
	cursor := ""
	for {
		keys, resp, err := index.ListKeysWithPrefixPaginated(ownershipPrefix, &pagination.PaginationRequest{Limit: 100, Cursor: cursor})
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
		if strings.Contains(key, fmt.Sprintf(":t:%s", threadID)) {
			if err := index.DeleteKey(key); err != nil {
				logger.Error("delete_ownership_relationship_failed", "key", key, "error", err)
			}
		}
	}

	participationPrefix := fmt.Sprintf("rel:t:%s:u:", threadID)
	participationKeys, _, err := index.ListKeysWithPrefixPaginated(participationPrefix, &pagination.PaginationRequest{Limit: 10000, Cursor: ""})
	if err != nil {
		return fmt.Errorf("list participation keys: %w", err)
	}

	for _, key := range participationKeys {
		if err := index.DeleteKey(key); err != nil {
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
