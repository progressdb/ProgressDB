package mi

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/db/indexdb"
	ki "progressdb/pkg/store/iterator/frontend/mi/ki"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

type MessageIterator struct {
	db *pebble.DB
	ki *ki.KeyIterator
}

func NewMessageIterator(db *pebble.DB) *MessageIterator {
	return &MessageIterator{
		db: db,
		ki: ki.NewKeyIterator(db),
	}
}

func (mi *MessageIterator) ExecuteMessageQuery(threadKey string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	if !keys.IsThreadKey(threadKey) {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("invalid thread key: %s", threadKey)
	}

	// Cache KI instance to avoid multiple database connections
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// Execute key query using the proven ki logic
	messageKeys, _, err := mi.ki.ExecuteKeyQuery(messagePrefix, req)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	// Batch lookup delete markers for all message keys
	deleteMarkers := mi.getDeleteMarkersWithFallback(messageKeys)

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
			// Before query: check if there are newer non-deleted messages between newest result and reference point
			hasNewer, _ := mi.checkHasMessages(threadKey, filteredMessageKeys[0], "before", req.Before)
			response.HasBefore = hasNewer
			// Check if there are older messages between oldest result and reference point
			hasOlder, _ := mi.checkHasMessages(threadKey, filteredMessageKeys[len(filteredMessageKeys)-1], "after", req.Before)
			response.HasAfter = hasOlder
		} else if req.After != "" {
			// After query: check if there are older non-deleted messages
			response.HasBefore = true // There are newer messages than what we're showing
			hasOlder, _ := mi.checkHasMessages(threadKey, filteredMessageKeys[len(filteredMessageKeys)-1], "after")
			response.HasAfter = hasOlder
		} else if req.Anchor != "" {
			// Anchor query: check both sides
			hasNewer, _ := mi.checkHasMessages(threadKey, filteredMessageKeys[0], "before")
			hasOlder, _ := mi.checkHasMessages(threadKey, filteredMessageKeys[len(filteredMessageKeys)-1], "after")
			response.HasBefore = hasNewer
			response.HasAfter = hasOlder
		}
	}

	// Set anchors based on filtered message keys
	mi.setAnchors(&response, filteredMessageKeys, req)

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

// getDeleteMarkersWithFallback performs delete marker lookup with individual fallback
func (mi *MessageIterator) getDeleteMarkersWithFallback(messageKeys []string) map[string]bool {
	deleteMarkers, err := mi.batchGetDeleteMarkers(messageKeys)
	if err != nil {
		// If batch lookup fails, fall back to individual checks
		deleteMarkers = make(map[string]bool)
		for _, messageKey := range messageKeys {
			messageDeleted, checkErr := mi.isMessageDeleted(messageKey)
			if checkErr != nil {
				deleteMarkers[messageKey] = false // fail-safe
			} else {
				deleteMarkers[messageKey] = messageDeleted
			}
		}
	}
	return deleteMarkers
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

	// Use frontend ki for consistent counting with smaller batch size for efficiency
	messagePrefix, err = keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return 0, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// Use integrated logic for consistent counting with smaller batch size for efficiency
	count := 0
	batchSize := 10000

	for {
		// Process in batches to avoid loading all keys at once
		messageKeys, _, err := mi.ki.ExecuteKeyQuery(messagePrefix, pagination.PaginationRequest{
			Limit: batchSize,
		})
		if err != nil {
			return 0, err
		}

		if len(messageKeys) == 0 {
			break // No more keys
		}

		// Batch lookup delete markers for this batch
		deleteMarkers := mi.getDeleteMarkersWithFallback(messageKeys)

		// Count non-deleted messages in this batch
		for _, messageKey := range messageKeys {
			if !deleteMarkers[messageKey] {
				count++
			}
		}

		// If we got fewer keys than batch size, we're done
		if len(messageKeys) < batchSize {
			break
		}
	}

	return count, nil
}

// checkHasMessages checks for non-deleted messages in a direction with optional boundary
func (mi *MessageIterator) checkHasMessages(threadKey, reference, direction string, boundary ...string) (bool, error) {
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return false, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// Build pagination request
	req := pagination.PaginationRequest{
		Limit: 10, // Small batch to check for messages
	}

	switch direction {
	case "before":
		req.Before = reference
	case "after":
		req.After = reference
	default:
		return false, fmt.Errorf("invalid direction: %s", direction)
	}

	// Use cached KI instance to get keys in the specified direction
	keys, _, err := mi.ki.ExecuteKeyQuery(messagePrefix, req)
	if err != nil {
		return false, err
	}

	// Get delete markers for the keys
	deleteMarkers := mi.getDeleteMarkersWithFallback(keys)

	// Check if any keys are non-deleted and respect boundary if provided
	for _, key := range keys {
		if !deleteMarkers[key] {
			// If boundary is provided, check if key is within bounds
			if len(boundary) > 0 {
				switch direction {
				case "before":
					if key > boundary[0] {
						return true, nil // Found newer non-deleted message after boundary
					}
				case "after":
					if key < boundary[0] {
						return true, nil // Found older non-deleted message before boundary
					}
				}
			} else {
				return true, nil // Found non-deleted message
			}
		}
	}

	return false, nil // No non-deleted messages found
}

// setAnchors sets pagination anchors based on query type and filtered keys
func (mi *MessageIterator) setAnchors(response *pagination.PaginationResponse, filteredMessageKeys []string, req pagination.PaginationRequest) {
	if len(filteredMessageKeys) == 0 {
		return
	}

	if req.Before != "" {
		// Before query: keys are newest→oldest
		response.AfterAnchor = filteredMessageKeys[0]
		response.BeforeAnchor = filteredMessageKeys[len(filteredMessageKeys)-1]
	} else if req.After != "" {
		// After query: keys are oldest→newest
		response.BeforeAnchor = filteredMessageKeys[0]
		response.AfterAnchor = filteredMessageKeys[len(filteredMessageKeys)-1]
	} else if req.Anchor != "" {
		// Anchor query: keys are oldest→newest (sorted by sorter)
		response.BeforeAnchor = filteredMessageKeys[0]
		response.AfterAnchor = filteredMessageKeys[len(filteredMessageKeys)-1]
	} else {
		// Initial load: keys are newest→oldest
		response.BeforeAnchor = filteredMessageKeys[0]
		response.AfterAnchor = filteredMessageKeys[len(filteredMessageKeys)-1]
	}
}
