package ti

import (
	"fmt"
	"log"
	"strings"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/iterator/frontend/ki"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

type ThreadIterator struct {
	db *pebble.DB
	ki *ki.KeyIterator // Cached KI instance for efficient database operations
}

func NewThreadIterator(db *pebble.DB) *ThreadIterator {
	return &ThreadIterator{
		db: db,
		ki: ki.NewKeyIterator(db), // Cache KI instance for reuse
	}
}

func (ti *ThreadIterator) ExecuteThreadQuery(userID string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	log.Printf("[TI] ExecuteThreadQuery - userID: %s, before: %s, after: %s, anchor: %s, limit: %d",
		userID, req.Before, req.After, req.Anchor, req.Limit)

	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to generate user thread prefix: %w", err)
	}

	// Transform thread keys to relationship keys for pagination
	originalBefore := req.Before
	originalAfter := req.After
	originalAnchor := req.Anchor

	if req.Before != "" {
		threadTS := strings.TrimPrefix(req.Before, "t:")
		req.Before = fmt.Sprintf(keys.RelUserOwnsThread, userID, threadTS)
		log.Printf("[TI] Transformed before: %s -> %s", originalBefore, req.Before)
	}
	if req.After != "" {
		threadTS := strings.TrimPrefix(req.After, "t:")
		req.After = fmt.Sprintf(keys.RelUserOwnsThread, userID, threadTS)
		log.Printf("[TI] Transformed after: %s -> %s", originalAfter, req.After)
	}
	if req.Anchor != "" {
		threadTS := strings.TrimPrefix(req.Anchor, "t:")
		req.Anchor = fmt.Sprintf(keys.RelUserOwnsThread, userID, threadTS)
		log.Printf("[TI] Transformed anchor: %s -> %s", originalAnchor, req.Anchor)
	}

	// Execute key query using the cached KI instance
	relationshipKeys, _, err := ti.ki.ExecuteKeyQuery(userThreadPrefix, req)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	log.Printf("[TI] KI returned %d relationship keys", len(relationshipKeys))
	for i, relKey := range relationshipKeys {
		if i < 5 { // Log first 5 to avoid spam
			log.Printf("[TI] RelKey[%d]: %s", i, relKey)
		}
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

	// Get delete markers using the consolidated function
	deleteMarkers := ti.getDeleteMarkersWithFallback(threadKeys)

	// Filter out deleted threads BEFORE setting pagination metadata
	filteredRelKeys := make([]string, 0, len(relationshipKeys))
	deletedCount := 0
	for _, relKey := range relationshipKeys {
		threadKey := relKeyToThreadKeyMap[relKey]
		if threadKey == "" {
			continue
		}

		// Only include non-deleted threads
		if !deleteMarkers[threadKey] {
			filteredRelKeys = append(filteredRelKeys, relKey)
		} else {
			deletedCount++
		}
	}

	log.Printf("[TI] After filtering: %d threads kept, %d deleted", len(filteredRelKeys), deletedCount)
	for i, relKey := range filteredRelKeys {
		if i < 5 { // Log first 5 to avoid spam
			threadKey := relKeyToThreadKeyMap[relKey]
			log.Printf("[TI] FilteredRelKey[%d]: %s -> thread: %s", i, relKey, threadKey)
		}
	}

	// Get total thread count (excluding deleted)
	total, err := ti.getTotalThreadCount(userID)
	if err != nil {
		total = 0
	}

	log.Printf("[TI] Total thread count: %d", total)

	// Create new response with correct counts based on filtered data
	response := pagination.PaginationResponse{
		Count: len(filteredRelKeys),
		Total: total,
	}

	// Use helper functions for accurate pagination based on filtered data
	if len(filteredRelKeys) == 0 {
		response.HasBefore = false
		response.HasAfter = false
		log.Printf("[TI] No filtered threads - HasBefore: false, HasAfter: false")
	} else {
		// For initial loads (no before/after params)
		if req.Before == "" && req.After == "" && req.Anchor == "" {
			response.HasBefore = false // We're starting from newest
			response.HasAfter = len(filteredRelKeys) < total
			log.Printf("[TI] Initial load - HasBefore: false, HasAfter: %t (count: %d < total: %d)",
				response.HasAfter, len(filteredRelKeys), total)
		} else if req.Before != "" {
			// Before query: we're starting from reference and moving backwards
			response.HasBefore = false // No threads newer than our starting reference
			log.Printf("[TI] Before query - HasBefore: false (starting from reference)")

			// Check if there are older threads beyond the oldest result
			oldestRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			log.Printf("[TI] Before query - checking older threads from: %s", oldestRelKey)
			hasOlder, _ := ti.checkHasOlderThreads(userID, oldestRelKey)
			response.HasAfter = hasOlder
			log.Printf("[TI] Before query - HasAfter (older): %t", hasOlder)
		} else if req.After != "" {
			// After query: check if there are older non-deleted threads
			response.HasBefore = true // There are newer threads than what we're showing
			oldestRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			log.Printf("[TI] After query - checking older threads from: %s", oldestRelKey)
			hasOlder, _ := ti.checkHasOlderThreads(userID, oldestRelKey)
			response.HasAfter = hasOlder
			log.Printf("[TI] After query - HasBefore: true, HasAfter (older): %t", hasOlder)
		} else if req.Anchor != "" {
			// Anchor query: check both directions
			newestRelKey := filteredRelKeys[0]
			oldestRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			hasNewer, _ := ti.checkHasNewerThreads(userID, newestRelKey)
			hasOlder, _ := ti.checkHasOlderThreads(userID, oldestRelKey)
			response.HasBefore = hasNewer
			response.HasAfter = hasOlder
			log.Printf("[TI] Anchor query - HasBefore: %t, HasAfter: %t", hasNewer, hasOlder)
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
				log.Printf("[TI] Before query - AfterAnchor: %s", parsed.ThreadKey)
			}
			if parsed, err := keys.ParseUserOwnsThread(lastRelKey); err == nil {
				response.BeforeAnchor = parsed.ThreadKey
				log.Printf("[TI] Before query - BeforeAnchor: %s", parsed.ThreadKey)
			}
		} else if req.After != "" {
			// After query: keys are oldest→newest
			firstRelKey := filteredRelKeys[0]
			lastRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			if parsed, err := keys.ParseUserOwnsThread(firstRelKey); err == nil {
				response.BeforeAnchor = parsed.ThreadKey
				log.Printf("[TI] After query - BeforeAnchor: %s", parsed.ThreadKey)
			}
			if parsed, err := keys.ParseUserOwnsThread(lastRelKey); err == nil {
				response.AfterAnchor = parsed.ThreadKey
				log.Printf("[TI] After query - AfterAnchor: %s", parsed.ThreadKey)
			}
		} else {
			// Initial load: keys are newest→oldest
			firstRelKey := filteredRelKeys[0]
			lastRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			if parsed, err := keys.ParseUserOwnsThread(firstRelKey); err == nil {
				response.BeforeAnchor = parsed.ThreadKey
				log.Printf("[TI] Initial load - BeforeAnchor: %s", parsed.ThreadKey)
			}
			if parsed, err := keys.ParseUserOwnsThread(lastRelKey); err == nil {
				response.AfterAnchor = parsed.ThreadKey
				log.Printf("[TI] Initial load - AfterAnchor: %s", parsed.ThreadKey)
			}
		}
	}

	log.Printf("[TI] Final response - Count: %d, Total: %d, HasBefore: %t, HasAfter: %t, BeforeAnchor: %s, AfterAnchor: %s",
		response.Count, response.Total, response.HasBefore, response.HasAfter, response.BeforeAnchor, response.AfterAnchor)

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
	log.Printf("[TI] getTotalThreadCount - starting count for userID: %s", userID)

	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return 0, err
	}

	// Use cached KI instance for efficient counting
	const batchSize = 1000
	totalCount := 0
	batchCount := 0
	offset := ""

	for {
		batchCount++

		// Get batch of relationship keys
		req := pagination.PaginationRequest{
			Limit: batchSize,
		}
		if offset != "" {
			req.After = offset
		}

		relationshipKeys, _, err := ti.ki.ExecuteKeyQuery(userThreadPrefix, req)
		if err != nil {
			log.Printf("[TI] getTotalThreadCount - batch %d failed: %v", batchCount, err)
			return totalCount, err
		}

		if len(relationshipKeys) == 0 {
			log.Printf("[TI] getTotalThreadCount - no more threads after batch %d", batchCount)
			break // No more threads
		}

		log.Printf("[TI] getTotalThreadCount - batch %d: %d relationship keys", batchCount, len(relationshipKeys))

		// Extract thread keys from relationship keys
		threadKeys := make([]string, 0, len(relationshipKeys))
		for _, relKey := range relationshipKeys {
			if parsed, err := keys.ParseUserOwnsThread(relKey); err == nil {
				threadKeys = append(threadKeys, parsed.ThreadKey)
			}
		}

		// Get delete markers for this batch
		deleteMarkers := ti.getDeleteMarkersWithFallback(threadKeys)

		// Count non-deleted threads in this batch
		batchNonDeleted := 0
		for _, threadKey := range threadKeys {
			if !deleteMarkers[threadKey] {
				totalCount++
				batchNonDeleted++
			}
		}

		log.Printf("[TI] getTotalThreadCount - batch %d: %d non-deleted threads, total so far: %d",
			batchCount, batchNonDeleted, totalCount)

		// If we got fewer than requested, we're done
		if len(relationshipKeys) < batchSize {
			break
		}

		// Set offset for next batch
		offset = relationshipKeys[len(relationshipKeys)-1]
	}

	log.Printf("[TI] getTotalThreadCount - final count: %d", totalCount)
	return totalCount, nil
}

// checkHasThreads checks for non-deleted threads in a direction with optional boundary
func (ti *ThreadIterator) checkHasThreads(userID, reference, direction string, boundary ...string) bool {
	// Validate reference key
	_, err := keys.ParseUserOwnsThread(reference)
	if err != nil {
		log.Printf("[TI] checkHasThreads - invalid reference key: %s", reference)
		return false
	}

	// Use cached KI instance
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		log.Printf("[TI] checkHasThreads - failed to generate prefix: %v", err)
		return false
	}

	// Build pagination request
	req := pagination.PaginationRequest{
		Limit: 10, // Small batch to check for threads
	}

	switch direction {
	case "before":
		req.Before = reference
	case "after":
		req.After = reference
	default:
		log.Printf("[TI] checkHasThreads - invalid direction: %s", direction)
		return false
	}

	// Get relationship keys in the specified direction
	relKeys, _, err := ti.ki.ExecuteKeyQuery(userThreadPrefix, req)
	if err != nil {
		log.Printf("[TI] checkHasThreads - KI query failed: %v", err)
		return false
	}

	log.Printf("[TI] checkHasThreads - KI returned %d relKeys in %s direction", len(relKeys), direction)
	for i, relKey := range relKeys {
		if i < 3 { // Log first 3 to avoid spam
			log.Printf("[TI] checkHasThreads - relKey[%d]: %s", i, relKey)
		}
	}

	// Extract thread keys from relationship keys
	threadKeys := make([]string, 0, len(relKeys))
	for _, relKey := range relKeys {
		if parsed, err := keys.ParseUserOwnsThread(relKey); err == nil {
			threadKeys = append(threadKeys, parsed.ThreadKey)
		}
	}

	// Get delete markers for thread keys
	deleteMarkers := ti.getDeleteMarkersWithFallback(threadKeys)

	// Check if any threads are non-deleted and respect boundary if provided
	for _, threadKey := range threadKeys {
		if !deleteMarkers[threadKey] {
			// If boundary is provided, check if key is within bounds and exclude the boundary itself
			if len(boundary) > 0 {
				switch direction {
				case "before":
					if threadKey > boundary[0] {
						log.Printf("[TI] checkHasThreads - found newer non-deleted thread after boundary: %s > %s", threadKey, boundary[0])
						return true // Found newer non-deleted thread after boundary
					}
				case "after":
					if threadKey < boundary[0] {
						log.Printf("[TI] checkHasThreads - found older non-deleted thread before boundary: %s < %s", threadKey, boundary[0])
						return true // Found older non-deleted thread before boundary
					}
				}
			} else {
				log.Printf("[TI] checkHasThreads - found non-deleted thread: %s", threadKey)
				return true // Found non-deleted thread
			}
		}
	}

	log.Printf("[TI] checkHasThreads - no non-deleted threads found")
	return false // No non-deleted threads found
}

// checkHasNewerThreads checks if there are newer non-deleted threads after the newest filtered key
func (ti *ThreadIterator) checkHasNewerThreads(userID, newestKey string) (bool, error) {
	return ti.checkHasThreads(userID, newestKey, "before"), nil
}

// checkHasOlderThreads checks if there are older non-deleted threads before the oldest filtered key
func (ti *ThreadIterator) checkHasOlderThreads(userID, oldestKey string) (bool, error) {
	return ti.checkHasThreads(userID, oldestKey, "after"), nil
}

// getDeleteMarkersWithFallback performs efficient batch lookup of delete markers with fallback to individual checks
func (ti *ThreadIterator) getDeleteMarkersWithFallback(threadKeys []string) map[string]bool {
	deleteMarkers := make(map[string]bool)

	if len(threadKeys) == 0 {
		return deleteMarkers
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

	return deleteMarkers
}
