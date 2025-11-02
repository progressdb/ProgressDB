package threads

import (
	"encoding/json"
	"fmt"
	"strings"

	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

func PurgeThreadPermanently(threadKey string) error {
	if threadKey == "" {
		return fmt.Errorf("threadKey cannot be empty")
	}

	// Store: delete all messages in thread
	if err := deleteAllMessagesInThread(threadKey); err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}

	// Store: delete the thread data
	if err := deleteThreadData(threadKey); err != nil {
		return fmt.Errorf("failed to delete thread data: %w", err)
	}

	// Index: delete thread message indexes
	if err := indexdb.DeleteThreadMessageIndexes(threadKey); err != nil {
		logger.Error("delete_thread_message_indexes_failed", "thread", threadKey, "error", err)
	}

	// Index: delete all thread indexes (including any remaining indexes)
	if err := DeleteAllThreadIndexes(threadKey); err != nil {
		logger.Error("delete_all_thread_indexes_failed", "thread", threadKey, "error", err)
	}

	// Delete the soft delete marker
	if err := indexdb.DeleteSoftDeleteMarker(threadKey); err != nil {
		logger.Error("unmark_thread_soft_deleted_purge_failed", "thread", threadKey, "error", err)
	}

	logger.Info("purge_thread_completed", "thread", threadKey)
	return nil
}

func deleteAllMessagesInThread(threadKey string) error {
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

	// First pass: collect all message keys and their version keys
	var messageKeys []string
	var versionKeys []string

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		messageKeys = append(messageKeys, key)

		// If this is a message, check if it has versions and collect them
		if strings.Contains(key, ":m:") {
			value := iter.Value()
			var m models.Message
			if err := json.Unmarshal(value, &m); err == nil && m.Key != "" {
				// Delete version indexes for this message
				if err := indexdb.DeleteVersionIndexes(threadKey, m.Key); err != nil {
					logger.Error("delete_version_indexes_failed", "thread", threadKey, "message", m.Key, "error", err)
				}

				versionPrefix, err := keys.GenAllMessageVersionsPrefix(m.Key)
				if err != nil {
					logger.Error("failed_to_generate_version_prefix", "error", err)
					continue
				}

				// Only look for versions if they might exist
				vIter, err := storedb.Client.NewIter(&pebble.IterOptions{
					LowerBound: []byte(versionPrefix),
					UpperBound: calculateUpperBound(versionPrefix),
				})
				if err == nil {
					// Collect all version keys if any exist
					for vIter.First(); vIter.Valid(); vIter.Next() {
						versionKeys = append(versionKeys, string(vIter.Key()))
					}
					vIter.Close()
				}
			}
		}
	}

	// Second pass: delete all version keys
	for _, versionKey := range versionKeys {
		if err := storedb.DeleteKey(versionKey); err != nil {
			logger.Error("delete_version_failed", "key", versionKey, "error", err)
		}
	}

	// Third pass: delete all message keys
	for _, messageKey := range messageKeys {
		if err := storedb.DeleteKey(messageKey); err != nil {
			logger.Error("delete_message_failed", "key", messageKey, "error", err)
		}
	}

	return nil
}

func deleteThreadData(threadKey string) error {
	threadDataKey := keys.GenThreadKey(threadKey)
	return storedb.DeleteKey(threadDataKey)
}

func DeleteAllThreadIndexes(threadKey string) error {
	if indexdb.Client == nil {
		return fmt.Errorf("pebble not opened; call Open first")
	}

	prefix := fmt.Sprintf("idx:t:%s:", threadKey)
	iter, err := indexdb.Client.NewIter(&pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: calculateUpperBound(prefix),
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	var keysToDelete [][]byte
	for iter.First(); iter.Valid(); iter.Next() {
		keysToDelete = append(keysToDelete, append([]byte(nil), iter.Key()...))
	}
	for _, k := range keysToDelete {
		if err := indexdb.DeleteKey(string(k)); err != nil {
			logger.Error("delete_thread_index_key_failed", "key", string(k), "error", err)
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
