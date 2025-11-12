package mi

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/db/indexdb"
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
	messageKeys, _, err := keyIter.ExecuteKeyQuery(messagePrefix, req)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	// Batch lookup delete markers for all message keys
	deleteMarkers, err := mi.batchGetDeleteMarkers(messageKeys)
	if err != nil {
		// If batch lookup fails, fall back to individual checks
		deleteMarkers = make(map[string]bool)
		for _, messageKey := range messageKeys {
			messageDeleted, checkErr := mi.isMessageDeleted(messageKey)
			if checkErr != nil {
				// If we can't determine deletion status, include message (fail-safe)
				deleteMarkers[messageKey] = false
			} else {
				deleteMarkers[messageKey] = messageDeleted
			}
		}
	}

	// Filter out deleted messages BEFORE setting pagination metadata
	filteredMessageKeys := make([]string, 0, len(messageKeys))
	for _, messageKey := range messageKeys {
		// Only include non-deleted messages
		if !deleteMarkers[messageKey] {
			filteredMessageKeys = append(filteredMessageKeys, messageKey)
		}
	}

	// Get total message count (excluding deleted)
	total, err := mi.GetMessageCountExcludingDeleted(threadKey)
	if err != nil {
		total = 0
	}

	// Create new response with correct counts based on filtered data
	response := pagination.PaginationResponse{
		Count: len(filteredMessageKeys),
		Total: total,
	}

	// Use helper functions for accurate pagination based on filtered data
	if len(filteredMessageKeys) == 0 {
		response.HasBefore = false
		response.HasAfter = false
	} else {
		// For initial loads (no before/after params)
		if req.Before == "" && req.After == "" && req.Anchor == "" {
			response.HasBefore = false // We're starting from newest
			response.HasAfter = len(filteredMessageKeys) < total
		} else if req.Before != "" {
			// Before query: check if there are newer non-deleted messages
			hasNewer, _ := mi.checkHasBefore(threadKey, filteredMessageKeys[0])
			response.HasBefore = hasNewer
			// Check if there are older messages by looking after the oldest key
			hasOlder, _ := mi.checkHasAfter(threadKey, filteredMessageKeys[len(filteredMessageKeys)-1])
			response.HasAfter = hasOlder
		} else if req.After != "" {
			// After query: check if there are older non-deleted messages
			response.HasBefore = true // There are newer messages than what we're showing
			hasOlder, _ := mi.checkHasAfter(threadKey, filteredMessageKeys[len(filteredMessageKeys)-1])
			response.HasAfter = hasOlder
		}
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

	// Recalculate pagination boundaries based on filtered data
	if len(filteredMessageKeys) == 0 {
		response.HasBefore = false
		response.HasAfter = false
	} else {
		// For initial loads (no before/after params)
		if req.Before == "" && req.After == "" && req.Anchor == "" {
			response.HasBefore = false // We're starting from newest
			response.HasAfter = len(filteredMessageKeys) < total
		} else if req.Before != "" {
			// Before query: check if there are newer non-deleted messages
			response.HasBefore = len(filteredMessageKeys) < total
			response.HasAfter = true // There are older messages than what we're showing
		} else if req.After != "" {
			// After query: check if there are older non-deleted messages
			response.HasBefore = true // There are newer messages than what we're showing
			response.HasAfter = len(filteredMessageKeys) < total
		}
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

// batchGetDeleteMarkers performs efficient batch lookup of delete markers for multiple message keys
func (mi *MessageIterator) batchGetDeleteMarkers(messageKeys []string) (map[string]bool, error) {
	deleteMarkers := make(map[string]bool)

	if len(messageKeys) == 0 {
		return deleteMarkers, nil
	}

	// Generate delete marker keys for all message keys
	deleteMarkerKeys := make([]string, 0, len(messageKeys))
	keyToOriginalMap := make(map[string]string) // deleteMarkerKey -> originalKey

	for _, messageKey := range messageKeys {
		deleteMarkerKey := keys.GenSoftDeleteMarkerKey(messageKey)
		deleteMarkerKeys = append(deleteMarkerKeys, deleteMarkerKey)
		keyToOriginalMap[deleteMarkerKey] = messageKey
	}

	// Batch lookup all delete markers
	for _, deleteMarkerKey := range deleteMarkerKeys {
		_, err := indexdb.GetKey(deleteMarkerKey)
		if err != nil {
			if indexdb.IsNotFound(err) {
				// No soft delete marker found = not deleted
				deleteMarkers[keyToOriginalMap[deleteMarkerKey]] = false
			} else {
				// Error checking for marker = fail-safe, consider not deleted
				deleteMarkers[keyToOriginalMap[deleteMarkerKey]] = false
			}
		} else {
			// Soft delete marker found = message is deleted
			deleteMarkers[keyToOriginalMap[deleteMarkerKey]] = true
		}
	}

	return deleteMarkers, nil
}

// isMessageDeleted checks if a message is marked as deleted using soft delete markers
func (mi *MessageIterator) isMessageDeleted(messageKey string) (bool, error) {
	// Generate soft delete marker key
	deleteMarkerKey := keys.GenSoftDeleteMarkerKey(messageKey)

	// Check if soft delete marker exists
	_, err := indexdb.GetKey(deleteMarkerKey)
	if err != nil {
		if indexdb.IsNotFound(err) {
			// No soft delete marker found = not deleted
			return false, nil
		}
		// Error checking for marker = fail-safe, consider not deleted
		return false, nil
	}

	// Soft delete marker found = message is deleted
	return true, nil
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

	// Batch lookup delete markers for all message keys
	deleteMarkers, err := mi.batchGetDeleteMarkers(messageKeys)
	if err != nil {
		// If batch lookup fails, fall back to individual checks
		deleteMarkers = make(map[string]bool)
		for _, messageKey := range messageKeys {
			messageDeleted, checkErr := mi.isMessageDeleted(messageKey)
			if checkErr != nil {
				// If we can't determine deletion status, count it (fail-safe)
				deleteMarkers[messageKey] = false
			} else {
				deleteMarkers[messageKey] = messageDeleted
			}
		}
	}

	// Filter out deleted messages
	count := 0
	for _, messageKey := range messageKeys {
		// Only count non-deleted messages
		if !deleteMarkers[messageKey] {
			count++
		}
	}

	return count, nil
}

// checkHasBefore checks if there are newer non-deleted messages before the reference key
func (mi *MessageIterator) checkHasBefore(threadKey, reference string) (bool, error) {
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return false, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// Use KI to get a small batch of keys before the reference
	keyIter := ki.NewKeyIterator(mi.db)
	keysBefore, _, err := keyIter.ExecuteKeyQuery(messagePrefix, pagination.PaginationRequest{
		Before: reference,
		Limit:  10, // Small batch to check for newer messages
	})
	if err != nil {
		return false, err
	}

	// Batch lookup delete markers for keys before
	deleteMarkers, err := mi.batchGetDeleteMarkers(keysBefore)
	if err != nil {
		// If batch lookup fails, fall back to individual checks
		deleteMarkers = make(map[string]bool)
		for _, key := range keysBefore {
			messageDeleted, checkErr := mi.isMessageDeleted(key)
			if checkErr != nil {
				deleteMarkers[key] = false // fail-safe
			} else {
				deleteMarkers[key] = messageDeleted
			}
		}
	}

	// Check if any of the keys before are non-deleted
	for _, key := range keysBefore {
		if !deleteMarkers[key] {
			return true, nil // Found newer non-deleted message
		}
	}

	return false, nil // No newer non-deleted messages found
}

// checkHasAfter checks if there are older non-deleted messages after the reference key
func (mi *MessageIterator) checkHasAfter(threadKey, reference string) (bool, error) {
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return false, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// Use KI to get a small batch of keys after the reference
	keyIter := ki.NewKeyIterator(mi.db)
	keysAfter, _, err := keyIter.ExecuteKeyQuery(messagePrefix, pagination.PaginationRequest{
		After: reference,
		Limit: 10, // Small batch to check for older messages
	})
	if err != nil {
		return false, err
	}

	// Batch lookup delete markers for keys after
	deleteMarkers, err := mi.batchGetDeleteMarkers(keysAfter)
	if err != nil {
		// If batch lookup fails, fall back to individual checks
		deleteMarkers = make(map[string]bool)
		for _, key := range keysAfter {
			messageDeleted, checkErr := mi.isMessageDeleted(key)
			if checkErr != nil {
				deleteMarkers[key] = false // fail-safe
			} else {
				deleteMarkers[key] = messageDeleted
			}
		}
	}

	// Check if any of the keys after are non-deleted
	for _, key := range keysAfter {
		if !deleteMarkers[key] {
			return true, nil // Found older non-deleted message
		}
	}

	return false, nil // No older non-deleted messages found
}
