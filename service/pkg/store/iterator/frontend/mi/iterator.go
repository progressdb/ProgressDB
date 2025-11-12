package mi

import (
	"encoding/json"
	"fmt"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/indexdb"
	message_store "progressdb/pkg/store/features/messages"
	"progressdb/pkg/store/iterator/frontend/ki"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

type MessageIterator struct {
	db *pebble.DB
}

func NewMessageIterator(db *pebble.DB) *MessageIterator {
	return &MessageIterator{db: db}
}

func (mi *MessageIterator) GetMessageCount(threadKey string) (int, error) {
	indexes, err := indexdb.GetThreadMessageIndexData(threadKey)
	if err != nil {
		return mi.countMessagesManually(threadKey)
	}

	return int(indexes.End), nil
}

func (mi *MessageIterator) countMessagesManually(threadKey string) (int, error) {
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return 0, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// Use frontend ki for consistent counting
	keyIter := ki.NewKeyIterator(mi.db)
	keys, _, err := keyIter.ExecuteKeyQuery(messagePrefix, pagination.PaginationRequest{Limit: 1000000})
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}

	return len(keys), nil
}

func (mi *MessageIterator) ExecuteMessageQuery(threadKey string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	if !keys.IsThreadKey(threadKey) {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("invalid thread key: %s", threadKey)
	}

	// Use the frontend key iterator for robust key iteration
	keyIter := ki.NewKeyIterator(mi.db)

	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// Execute key query using the proven ki logic
	messageKeys, kiResponse, err := keyIter.ExecuteKeyQuery(messagePrefix, req)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	// Filter out deleted messages BEFORE setting pagination metadata
	filteredMessageKeys := make([]string, 0, len(messageKeys))
	for _, messageKey := range messageKeys {
		// Check if message is deleted by fetching message data
		messageDeleted, err := mi.isMessageDeleted(messageKey)
		if err != nil {
			// If we can't determine deletion status, include message (fail-safe)
			filteredMessageKeys = append(filteredMessageKeys, messageKey)
			continue
		}

		// Only include non-deleted messages
		if !messageDeleted {
			filteredMessageKeys = append(filteredMessageKeys, messageKey)
		}
	}

	// Get total message count (excluding deleted)
	total, err := mi.GetMessageCountExcludingDeleted(threadKey)
	if err != nil {
		total = 0
	}

	// Create new response with correct counts and anchors based on filtered data
	response := pagination.PaginationResponse{
		HasBefore: kiResponse.HasBefore,
		HasAfter:  kiResponse.HasAfter,
		Count:     len(filteredMessageKeys),
		Total:     total,
	}

	// Set anchors based on filtered message keys
	if len(filteredMessageKeys) > 0 {
		if req.Before != "" {
			// Before query: keys are newest→oldest
			response.AfterAnchor = filteredMessageKeys[0]
			response.BeforeAnchor = filteredMessageKeys[len(filteredMessageKeys)-1]
		} else if req.After != "" {
			// After query: keys are oldest→newest
			response.BeforeAnchor = filteredMessageKeys[0]
			response.AfterAnchor = filteredMessageKeys[len(filteredMessageKeys)-1]
		} else {
			// Initial load: keys are newest→oldest
			response.BeforeAnchor = filteredMessageKeys[0]
			response.AfterAnchor = filteredMessageKeys[len(filteredMessageKeys)-1]
		}
	}

	var sortedKeys []string

	// Only sort for initial load - before/after are already in correct order from ki
	if req.Before == "" && req.After == "" && req.Anchor == "" {
		sorter := NewMessageSorter()
		sortedKeys = sorter.SortKeys(filteredMessageKeys, req.SortBy, &response)
	} else {
		// Use keys as-is from ki (already in correct order)
		sortedKeys = filteredMessageKeys
	}

	return sortedKeys, response, nil
}

// isMessageDeleted checks if a message is marked as deleted by fetching its data
func (mi *MessageIterator) isMessageDeleted(messageKey string) (bool, error) {
	messageData, err := message_store.GetMessageData(messageKey)
	if err != nil {
		// If message data doesn't exist or can't be fetched, consider it not deleted
		return false, nil
	}

	// Parse message JSON to check deleted status
	var message models.Message
	if err := json.Unmarshal([]byte(messageData), &message); err != nil {
		// If JSON is invalid, consider it not deleted (fail-safe)
		return false, nil
	}

	return message.Deleted, nil
}

// GetMessageCountExcludingDeleted returns the count of non-deleted messages in a thread
func (mi *MessageIterator) GetMessageCountExcludingDeleted(threadKey string) (int, error) {
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return 0, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// Use frontend ki for consistent counting
	keyIter := ki.NewKeyIterator(mi.db)
	messageKeys, _, err := keyIter.ExecuteKeyQuery(messagePrefix, pagination.PaginationRequest{Limit: 1000000})
	if err != nil {
		return 0, err
	}

	// Filter out deleted messages
	count := 0
	for _, messageKey := range messageKeys {
		messageDeleted, err := mi.isMessageDeleted(messageKey)
		if err != nil {
			// If we can't determine deletion status, count it (fail-safe)
			count++
			continue
		}

		// Only count non-deleted messages
		if !messageDeleted {
			count++
		}
	}

	return count, nil
}
