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

	// Index: delete the thread message indexes
	if err := indexdb.DeleteThreadMessageIndexes(threadKey); err != nil {
		logger.Error("delete_thread_message_indexes_failed", "thread", threadKey, "error", err)
	}

	// Get all thread <> user relationships and delete each
	if err := deleteThreadUserRelationships(threadKey); err != nil {
		logger.Error("delete_thread_user_relationships_failed", "thread", threadKey, "error", err)
	}

	// Delete the soft delete marker
	if err := indexdb.UnmarkSoftDeleted(threadKey); err != nil {
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
				versionPrefix, err := keys.GenAllMessageVersionsPrefix(m.Key)
				if err != nil {
					logger.Error("failed_to_generate_version_prefix", "error", err)
					continue
				}

				// Only look for versions if they might exist
				// Check if there are any version keys before iterating
				vIter, err := storedb.Client.NewIter(&pebble.IterOptions{
					LowerBound: []byte(versionPrefix),
					UpperBound: calculateUpperBound(versionPrefix),
				})
				if err == nil {
					// Only iterate if we find at least one version
					if vIter.First() && vIter.Valid() {
						for ; vIter.Valid(); vIter.Next() {
							versionKeys = append(versionKeys, string(vIter.Key()))
						}
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
	threadKey = keys.GenThreadKey(threadKey)
	return storedb.DeleteKey(threadKey)
}

func deleteThreadUserRelationships(threadKey string) error {
	// First, get all users that have a relationship with this thread
	participationPrefix, err := keys.GenThreadUserRelPrefix(threadKey)
	if err != nil {
		return fmt.Errorf("failed to generate participation prefix: %w", err)
	}

	// Collect all user IDs from thread <> user relationships
	var userIDs []string
	iter, err := indexdb.Client.NewIter(&pebble.IterOptions{
		LowerBound: []byte(participationPrefix),
		UpperBound: calculateUpperBound(participationPrefix),
	})
	if err != nil {
		return fmt.Errorf("failed to create participation iterator: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Parse the key to extract user ID: rel:t:{thread_key}:u:{user_id}
		parts := strings.Split(key, ":")
		if len(parts) >= 5 && parts[0] == "rel" && parts[1] == "t" && parts[3] == "u" {
			userID := parts[4]
			userIDs = append(userIDs, userID)
		}

		// Delete the thread <> user relationship
		if err := indexdb.DeleteKey(key); err != nil {
			logger.Error("delete_participation_relationship_failed", "key", key, "error", err)
		}
	}

	// Now delete the corresponding user <> thread ownership relationships for each user
	for _, userID := range userIDs {
		ownershipKey := keys.GenUserOwnsThreadKey(userID, threadKey)
		if err := indexdb.DeleteKey(ownershipKey); err != nil {
			logger.Error("delete_ownership_relationship_failed", "key", ownershipKey, "error", err)
		}
	}

	return nil
}

func deleteRelationshipsWithPrefix(prefix, threadKey string) error {
	iter, err := indexdb.Client.NewIter(&pebble.IterOptions{
		LowerBound: []byte(prefix),
	})
	if err != nil {
		return fmt.Errorf("failed to create ownership iterator: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if strings.Contains(key, fmt.Sprintf(":t:%s", threadKey)) {
			if err := indexdb.DeleteKey(key); err != nil {
				logger.Error("delete_ownership_relationship_failed", "key", key, "error", err)
			}
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
