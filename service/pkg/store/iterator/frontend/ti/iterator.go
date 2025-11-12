package ti

import (
	"fmt"
	"strings"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/iterator/frontend/ki"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

type ThreadIterator struct {
	db *pebble.DB
}

func NewThreadIterator(db *pebble.DB) *ThreadIterator {
	return &ThreadIterator{db: db}
}

func (ti *ThreadIterator) ExecuteThreadQuery(userID string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to generate user thread prefix: %w", err)
	}

	// Transform thread keys to relationship keys for pagination
	if req.Before != "" {
		threadTS := strings.TrimPrefix(req.Before, "t:")
		req.Before = fmt.Sprintf(keys.RelUserOwnsThread, userID, threadTS)
	}
	if req.After != "" {
		threadTS := strings.TrimPrefix(req.After, "t:")
		req.After = fmt.Sprintf(keys.RelUserOwnsThread, userID, threadTS)
	}
	if req.Anchor != "" {
		threadTS := strings.TrimPrefix(req.Anchor, "t:")
		req.Anchor = fmt.Sprintf(keys.RelUserOwnsThread, userID, threadTS)
	}

	// Use the frontend key iterator for robust key iteration
	keyIter := ki.NewKeyIterator(ti.db)

	// Execute key query using the proven ki logic
	relationshipKeys, _, err := keyIter.ExecuteKeyQuery(userThreadPrefix, req)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	// Extract thread keys from relationship keys for batch delete marker lookup
	threadKeys := make([]string, 0, len(relationshipKeys))
	relKeyToThreadKeyMap := make(map[string]string) // relKey -> threadKey

	for _, relKey := range relationshipKeys {
		parsed, err := keys.ParseUserOwnsThread(relKey)
		if err != nil {
			continue
		}
		threadKeys = append(threadKeys, parsed.ThreadKey)
		relKeyToThreadKeyMap[relKey] = parsed.ThreadKey
	}

	// Batch lookup delete markers for all thread keys
	deleteMarkers, batchErr := ti.batchGetDeleteMarkers(threadKeys)
	if batchErr != nil {
		// If batch lookup fails, fall back to individual checks
		deleteMarkers = make(map[string]bool)
		for _, threadKey := range threadKeys {
			threadDeleted, checkErr := ti.isThreadDeleted(threadKey)
			if checkErr != nil {
				// If we can't determine deletion status, include thread (fail-safe)
				deleteMarkers[threadKey] = false
			} else {
				deleteMarkers[threadKey] = threadDeleted
			}
		}
	}

	// Filter out deleted threads BEFORE setting pagination metadata
	filteredRelKeys := make([]string, 0, len(relationshipKeys))
	for _, relKey := range relationshipKeys {
		threadKey := relKeyToThreadKeyMap[relKey]
		if threadKey == "" {
			continue
		}

		// Only include non-deleted threads
		if !deleteMarkers[threadKey] {
			filteredRelKeys = append(filteredRelKeys, relKey)
		}
	}

	// Get total thread count (excluding deleted)
	total, err := ti.getTotalThreadCount(userID)
	if err != nil {
		total = 0
	}

	// Create new response with correct counts based on filtered data
	response := pagination.PaginationResponse{
		Count: len(filteredRelKeys),
		Total: total,
	}

	// Use helper functions for accurate pagination based on filtered data
	if len(filteredRelKeys) == 0 {
		response.HasBefore = false
		response.HasAfter = false
	} else {
		// For initial loads (no before/after params)
		if req.Before == "" && req.After == "" && req.Anchor == "" {
			response.HasBefore = false // We're starting from newest
			response.HasAfter = len(filteredRelKeys) < total
		} else if req.Before != "" {
			// Before query: check if there are newer non-deleted threads
			newestRelKey := filteredRelKeys[0]
			hasNewer, _ := ti.checkHasNewerThreads(userID, newestRelKey)
			response.HasBefore = hasNewer
			response.HasAfter = true // There are older threads than what we're showing
		} else if req.After != "" {
			// After query: check if there are older non-deleted threads
			response.HasBefore = true // There are newer threads than what we're showing
			oldestRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			hasOlder, _ := ti.checkHasOlderThreads(userID, oldestRelKey)
			response.HasAfter = hasOlder
		}
	}

	// Set anchors based on filtered relationship keys, but convert to thread keys
	if len(filteredRelKeys) > 0 {
		if req.Before != "" {
			// Before query: keys are newest→oldest
			firstRelKey := filteredRelKeys[0]
			lastRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			if parsed, err := keys.ParseUserOwnsThread(firstRelKey); err == nil {
				response.AfterAnchor = parsed.ThreadKey
			}
			if parsed, err := keys.ParseUserOwnsThread(lastRelKey); err == nil {
				response.BeforeAnchor = parsed.ThreadKey
			}
		} else if req.After != "" {
			// After query: keys are oldest→newest
			firstRelKey := filteredRelKeys[0]
			lastRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			if parsed, err := keys.ParseUserOwnsThread(firstRelKey); err == nil {
				response.BeforeAnchor = parsed.ThreadKey
			}
			if parsed, err := keys.ParseUserOwnsThread(lastRelKey); err == nil {
				response.AfterAnchor = parsed.ThreadKey
			}
		} else {
			// Initial load: keys are newest→oldest
			firstRelKey := filteredRelKeys[0]
			lastRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			if parsed, err := keys.ParseUserOwnsThread(firstRelKey); err == nil {
				response.BeforeAnchor = parsed.ThreadKey
			}
			if parsed, err := keys.ParseUserOwnsThread(lastRelKey); err == nil {
				response.AfterAnchor = parsed.ThreadKey
			}
		}
	}

	var sortedKeys []string

	// Only sort for initial load - before/after are already in correct order from ki
	if req.Before == "" && req.After == "" && req.Anchor == "" {
		sorter := NewThreadSorter()
		sortedKeys = sorter.SortKeys(filteredRelKeys, req.SortBy, &response)
	} else {
		// Use keys as-is from ki (already in correct order)
		sortedKeys = filteredRelKeys
	}

	// Convert filtered relationship keys to thread keys
	finalThreadKeys := make([]string, 0, len(sortedKeys))
	for _, relKey := range sortedKeys {
		parsed, err := keys.ParseUserOwnsThread(relKey)
		if err != nil {
			continue
		}
		finalThreadKeys = append(finalThreadKeys, parsed.ThreadKey)
	}

	return finalThreadKeys, response, nil
}

func (ti *ThreadIterator) getTotalThreadCount(userID string) (int, error) {
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return 0, err
	}

	// Use frontend ki for consistent counting
	keyIter := ki.NewKeyIterator(ti.db)
	relationshipKeys, _, err := keyIter.ExecuteKeyQuery(userThreadPrefix, pagination.PaginationRequest{Limit: 1000000})
	if err != nil {
		return 0, err
	}

	// Extract thread keys from relationship keys for batch delete marker lookup
	threadKeys := make([]string, 0, len(relationshipKeys))
	for _, relKey := range relationshipKeys {
		parsed, err := keys.ParseUserOwnsThread(relKey)
		if err != nil {
			continue
		}
		threadKeys = append(threadKeys, parsed.ThreadKey)
	}

	// Batch lookup delete markers for all thread keys
	deleteMarkers, err := ti.batchGetDeleteMarkers(threadKeys)
	if err != nil {
		// If batch lookup fails, fall back to individual checks
		deleteMarkers = make(map[string]bool)
		for _, threadKey := range threadKeys {
			threadDeleted, checkErr := ti.isThreadDeleted(threadKey)
			if checkErr != nil {
				// If we can't determine deletion status, count it (fail-safe)
				deleteMarkers[threadKey] = false
			} else {
				deleteMarkers[threadKey] = threadDeleted
			}
		}
	}

	// Filter out deleted threads
	count := 0
	for _, threadKey := range threadKeys {
		// Only count non-deleted threads
		if !deleteMarkers[threadKey] {
			count++
		}
	}

	return count, nil
}

// checkHasNewerThreads checks if there are newer non-deleted threads after the newest filtered key
func (ti *ThreadIterator) checkHasNewerThreads(userID, newestKey string) (bool, error) {
	// Convert thread key to relationship key for KI query
	_, err := keys.ParseUserOwnsThread(newestKey)
	if err != nil {
		return false, fmt.Errorf("failed to parse thread key: %w", err)
	}

	// Use KI to get relationship keys after the newest relationship key
	keyIter := ki.NewKeyIterator(ti.db)

	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return false, fmt.Errorf("failed to generate user thread prefix: %w", err)
	}

	// Get a small batch of relationship keys after newestKey to check for non-deleted threads
	relKeysAfter, _, err := keyIter.ExecuteKeyQuery(userThreadPrefix, pagination.PaginationRequest{
		After: newestKey,
		Limit: 10, // Small batch to check for newer threads
	})
	if err != nil {
		return false, err
	}

	// Extract thread keys from relationship keys
	threadKeys := make([]string, 0, len(relKeysAfter))
	for _, relKey := range relKeysAfter {
		if parsed, err := keys.ParseUserOwnsThread(relKey); err == nil {
			threadKeys = append(threadKeys, parsed.ThreadKey)
		}
	}

	// Batch lookup delete markers for thread keys
	deleteMarkers, err := ti.batchGetDeleteMarkers(threadKeys)
	if err != nil {
		// If batch lookup fails, fall back to individual checks
		deleteMarkers = make(map[string]bool)
		for _, threadKey := range threadKeys {
			threadDeleted, checkErr := ti.isThreadDeleted(threadKey)
			if checkErr != nil {
				deleteMarkers[threadKey] = false // fail-safe
			} else {
				deleteMarkers[threadKey] = threadDeleted
			}
		}
	}

	// Check if any of the threads after are non-deleted
	for _, threadKey := range threadKeys {
		if !deleteMarkers[threadKey] {
			return true, nil // Found newer non-deleted thread
		}
	}

	return false, nil // No newer non-deleted threads found
}

// checkHasOlderThreads checks if there are older non-deleted threads before the oldest filtered key
func (ti *ThreadIterator) checkHasOlderThreads(userID, oldestKey string) (bool, error) {
	// Convert thread key to relationship key for KI query
	parsed, err := keys.ParseUserOwnsThread(oldestKey)
	if err != nil {
		return false, fmt.Errorf("failed to parse thread key: %w", err)
	}

	// Use KI to get relationship keys before the oldest relationship key
	keyIter := ki.NewKeyIterator(ti.db)

	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return false, fmt.Errorf("failed to generate user thread prefix: %w", err)
	}

	// Get a small batch of relationship keys before oldestKey to check for non-deleted threads
	relKeysBefore, _, err := keyIter.ExecuteKeyQuery(userThreadPrefix, pagination.PaginationRequest{
		Before: oldestKey,
		Limit:  10, // Small batch to check for older threads
	})
	if err != nil {
		return false, err
	}

	// Extract thread keys from relationship keys
	threadKeys := make([]string, 0, len(relKeysBefore))
	for _, relKey := range relKeysBefore {
		if parsed, err := keys.ParseUserOwnsThread(relKey); err == nil {
			threadKeys = append(threadKeys, parsed.ThreadKey)
		}
		_ = parsed // avoid unused variable warning
	}

	// Batch lookup delete markers for thread keys
	deleteMarkers, err := ti.batchGetDeleteMarkers(threadKeys)
	if err != nil {
		// If batch lookup fails, fall back to individual checks
		deleteMarkers = make(map[string]bool)
		for _, threadKey := range threadKeys {
			threadDeleted, checkErr := ti.isThreadDeleted(threadKey)
			if checkErr != nil {
				deleteMarkers[threadKey] = false // fail-safe
			} else {
				deleteMarkers[threadKey] = threadDeleted
			}
		}
	}

	// Check if any of the threads before are non-deleted
	for _, threadKey := range threadKeys {
		if !deleteMarkers[threadKey] {
			return true, nil // Found older non-deleted thread
		}
	}

	return false, nil // No older non-deleted threads found
}

// batchGetDeleteMarkers performs efficient batch lookup of delete markers for multiple thread keys
func (ti *ThreadIterator) batchGetDeleteMarkers(threadKeys []string) (map[string]bool, error) {
	deleteMarkers := make(map[string]bool)

	if len(threadKeys) == 0 {
		return deleteMarkers, nil
	}

	// Generate delete marker keys for all thread keys
	deleteMarkerKeys := make([]string, 0, len(threadKeys))
	keyToOriginalMap := make(map[string]string) // deleteMarkerKey -> originalKey

	for _, threadKey := range threadKeys {
		deleteMarkerKey := keys.GenSoftDeleteMarkerKey(threadKey)
		deleteMarkerKeys = append(deleteMarkerKeys, deleteMarkerKey)
		keyToOriginalMap[deleteMarkerKey] = threadKey
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
			// Soft delete marker found = thread is deleted
			deleteMarkers[keyToOriginalMap[deleteMarkerKey]] = true
		}
	}

	return deleteMarkers, nil
}

// isThreadDeleted checks if a thread is marked as deleted using soft delete markers
func (ti *ThreadIterator) isThreadDeleted(threadKey string) (bool, error) {
	// Generate soft delete marker key
	deleteMarkerKey := keys.GenSoftDeleteMarkerKey(threadKey)

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

	// Soft delete marker found = thread is deleted
	return true, nil
}
