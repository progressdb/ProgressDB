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

	"github.com/cockroachdb/pebble"
)

// deletes thread and all messages/versions; removes in batches
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

	// Helper functions for batch deletion
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

	// Delete thread metadata first
	threadKey := keys.GenThreadKey(threadID)
	if err := storedb.Client.Delete([]byte(threadKey), storedb.WriteOpt(true)); err != nil {
		logger.Error("delete_thread_meta_failed", "thread", threadID, "error", err)
		return fmt.Errorf("failed to delete thread metadata: %w", err)
	} else {
		deletedKeys++
	}

	// Get thread prefix for efficient iteration of messages
	threadPrefix := keys.GenAllThreadMessagesPrefix(threadID)

	// Set up iterator bounds for efficient scanning
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

	// Collect all message keys and their versions
	for iter.First(); iter.Valid(); iter.Next() {
		key := append([]byte(nil), iter.Key()...)
		mainBatch = append(mainBatch, key)

		// If this is a message, collect its versions
		if bytes.Contains(key, []byte(":m:")) {
			value := append([]byte(nil), iter.Value()...)
			var m models.Message
			if err := json.Unmarshal(value, &m); err == nil && m.Key != "" {
				// Get version prefix for this message
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

	// Delete remaining batches
	if len(mainBatch) > 0 {
		deleteMainBatch(mainBatch)
	}
	if len(versionKeys) > 0 {
		deleteVersionBatch(versionKeys)
	}

	// Delete thread message indexes
	if err := index.DeleteThreadMessageIndexes(threadID); err != nil {
		logger.Error("delete_thread_message_indexes_failed", "thread", threadID, "error", err)
		// Continue with purge
	}

	// Remove soft delete marker
	if err := index.UnmarkSoftDeleted(threadID); err != nil {
		logger.Error("unmark_thread_soft_deleted_purge_failed", "thread", threadID, "error", err)
		// Continue with purge
	}

	// Remove relationship keys for this thread
	if err := cleanupThreadRelationships(threadID); err != nil {
		logger.Error("cleanup_thread_relationships_failed", "thread", threadID, "error", err)
		// Continue with purge
	}

	logger.Info("purge_thread_completed", "thread", threadID, "deleted_keys", deletedKeys)
	return nil
}

// cleanupThreadRelationships removes all relationship keys for a thread during permanent deletion
func cleanupThreadRelationships(threadID string) error {
	// Remove ownership relationships: rel:u:*:t:<threadID>
	ownershipPrefix := fmt.Sprintf("rel:u:")
	ownershipKeys, err := index.ListKeys(ownershipPrefix)
	if err != nil {
		return fmt.Errorf("list ownership keys: %w", err)
	}

	for _, key := range ownershipKeys {
		// Check if this key is for our thread: rel:u:<userID>:t:<threadID>
		if strings.Contains(key, fmt.Sprintf(":t:%s", threadID)) {
			if err := index.DeleteKey(key); err != nil {
				logger.Error("delete_ownership_relationship_failed", "key", key, "error", err)
			}
		}
	}

	// Remove participation relationships: rel:t:<threadID>:u:*
	participationPrefix := fmt.Sprintf("rel:t:%s:u:", threadID)
	participationKeys, err := index.ListKeys(participationPrefix)
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

// calculateUpperBound calculates upper bound for prefix iteration
func calculateUpperBound(prefix string) []byte {
	prefixBytes := []byte(prefix)
	upper := make([]byte, len(prefixBytes))
	copy(upper, prefixBytes)

	// Increment the last byte to get the upper bound
	for i := len(upper) - 1; i >= 0; i-- {
		if upper[i] < 0xFF {
			upper[i]++
			return upper
		}
		upper[i] = 0
	}

	// If we overflowed, return a prefix that will never match
	return append(prefixBytes, 0xFF)
}
