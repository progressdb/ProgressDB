package ti

import (
	"fmt"
	"strings"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/iterator/frontend/ki"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
)

type ThreadIterator struct {
	db *pebble.DB
	ki *ki.KeyIterator
}

func NewThreadIterator(db *pebble.DB) *ThreadIterator {
	return &ThreadIterator{
		db: db,
		ki: ki.NewKeyIterator(db),
	}
}

func (ti *ThreadIterator) ExecuteThreadQuery(userID string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	// Generate user thread prefix for database queries
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

	// Execute key query using cached KI instance
	relationshipKeys, _, err := ti.ki.ExecuteKeyQuery(userThreadPrefix, req)
	if err != nil {
		logger.Error("ThreadIterator key query failed", "userID", userID, "error", err)
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	logger.Debug("ThreadIterator query executed",
		"userID", userID,
		"before", req.Before,
		"after", req.After,
		"anchor", req.Anchor,
		"rawResults", len(relationshipKeys))

	threadKeys := make([]string, 0, len(relationshipKeys))
	relKeyToThreadKeyMap := make(map[string]string)
	for _, relKey := range relationshipKeys {
		parsed, err := keys.ParseUserOwnsThread(relKey)
		if err != nil {
			continue
		}
		threadKeys = append(threadKeys, parsed.ThreadKey)
		relKeyToThreadKeyMap[relKey] = parsed.ThreadKey
	}

	// Get delete markers for all threads in batch
	deleteMarkers := ti.getDeleteMarkersWithFallback(threadKeys)

	// Filter out deleted threads - only keep non-deleted threads
	filteredRelKeys := make([]string, 0, len(relationshipKeys))
	for _, relKey := range relationshipKeys {
		threadKey := relKeyToThreadKeyMap[relKey]
		if threadKey == "" {
			continue
		}
		if !deleteMarkers[threadKey] {
			filteredRelKeys = append(filteredRelKeys, relKey)
		}
	}

	logger.Debug("ThreadIterator delete filtering applied",
		"rawResults", len(relationshipKeys),
		"filteredResults", len(filteredRelKeys),
		"deletedCount", len(relationshipKeys)-len(filteredRelKeys))

	total, err := ti.getTotalThreadCount(userID)
	if err != nil {
		total = 0
	}

	response := pagination.PaginationResponse{
		Count: len(filteredRelKeys),
		Total: total,
	}

	// Set pagination flags based on query type and filtered results
	if len(filteredRelKeys) == 0 {
		// No results: no pagination possible
		response.HasBefore = false
		response.HasAfter = false
	} else {
		// Determine query type and set appropriate pagination flags
		if req.Before == "" && req.After == "" && req.Anchor == "" {
			// Initial load: start from newest, check if there are older threads
			response.HasBefore = false                       // We're at the newest threads
			response.HasAfter = len(filteredRelKeys) < total // More threads exist if count < total
		} else if req.Before != "" {
			// Before query: get threads newer than reference, check for older threads
			response.HasBefore = false // No threads newer than our reference point
			oldestRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			hasOlder, _ := ti.checkHasOlderThreads(userID, oldestRelKey)
			response.HasAfter = hasOlder
		} else if req.After != "" {
			// After query: get threads older than reference, check for even older threads
			response.HasBefore = true // There are newer threads than what we're showing
			oldestRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			hasOlder, _ := ti.checkHasOlderThreads(userID, oldestRelKey)
			response.HasAfter = hasOlder
		} else if req.Anchor != "" {
			// Anchor query: get threads around reference, check both directions
			newestRelKey := filteredRelKeys[0]
			oldestRelKey := filteredRelKeys[len(filteredRelKeys)-1]
			hasNewer, _ := ti.checkHasNewerThreads(userID, newestRelKey)
			hasOlder, _ := ti.checkHasOlderThreads(userID, oldestRelKey)
			response.HasBefore = hasNewer
			response.HasAfter = hasOlder
		}
	}

	// Set anchor points for next/previous pagination based on query type
	if len(filteredRelKeys) > 0 {
		if req.Before != "" {
			// Before query: results are newest→oldest
			firstRelKey := filteredRelKeys[0]                     // newest in this page
			lastRelKey := filteredRelKeys[len(filteredRelKeys)-1] // oldest in this page
			if parsed, err := keys.ParseUserOwnsThread(firstRelKey); err == nil {
				response.AfterAnchor = parsed.ThreadKey // For getting newer threads
			}
			if parsed, err := keys.ParseUserOwnsThread(lastRelKey); err == nil {
				response.BeforeAnchor = parsed.ThreadKey // For getting older threads
			}
		} else if req.After != "" {
			// After query: results are oldest→newest
			firstRelKey := filteredRelKeys[0]                     // oldest in this page
			lastRelKey := filteredRelKeys[len(filteredRelKeys)-1] // newest in this page
			if parsed, err := keys.ParseUserOwnsThread(firstRelKey); err == nil {
				response.BeforeAnchor = parsed.ThreadKey // For getting older threads
			}
			if parsed, err := keys.ParseUserOwnsThread(lastRelKey); err == nil {
				response.AfterAnchor = parsed.ThreadKey // For getting newer threads
			}
		} else {
			// Initial load: results are newest→oldest
			firstRelKey := filteredRelKeys[0]                     // newest overall
			lastRelKey := filteredRelKeys[len(filteredRelKeys)-1] // oldest in this page
			if parsed, err := keys.ParseUserOwnsThread(firstRelKey); err == nil {
				response.BeforeAnchor = parsed.ThreadKey // For getting older threads
			}
			if parsed, err := keys.ParseUserOwnsThread(lastRelKey); err == nil {
				response.AfterAnchor = parsed.ThreadKey // For getting newer threads
			}
		}
	}

	// Log final pagination metadata for debugging
	logger.Debug("ThreadIterator pagination metadata set",
		"hasBefore", response.HasBefore,
		"hasAfter", response.HasAfter,
		"beforeAnchor", response.BeforeAnchor,
		"afterAnchor", response.AfterAnchor,
		"count", response.Count,
		"total", response.Total)

	var sortedKeys []string
	if req.Before == "" && req.After == "" && req.Anchor == "" {
		sorter := NewThreadSorter()
		sortedKeys = sorter.SortKeys(filteredRelKeys, req.SortBy, &response)
	} else {
		sortedKeys = filteredRelKeys
	}

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

	const batchSize = 1000
	totalCount := 0
	offset := ""

	for {
		req := pagination.PaginationRequest{
			Limit: batchSize,
		}
		if offset != "" {
			req.After = offset
		}

		relationshipKeys, _, err := ti.ki.ExecuteKeyQuery(userThreadPrefix, req)
		if err != nil {
			return totalCount, err
		}

		if len(relationshipKeys) == 0 {
			break
		}

		threadKeys := make([]string, 0, len(relationshipKeys))
		for _, relKey := range relationshipKeys {
			if parsed, err := keys.ParseUserOwnsThread(relKey); err == nil {
				threadKeys = append(threadKeys, parsed.ThreadKey)
			}
		}

		deleteMarkers := ti.getDeleteMarkersWithFallback(threadKeys)

		for _, threadKey := range threadKeys {
			if !deleteMarkers[threadKey] {
				totalCount++
			}
		}

		if len(relationshipKeys) < batchSize {
			break
		}

		offset = relationshipKeys[len(relationshipKeys)-1]
	}

	return totalCount, nil
}

func (ti *ThreadIterator) checkHasThreads(userID, reference, direction string, boundary ...string) bool {
	_, err := keys.ParseUserOwnsThread(reference)
	if err != nil {
		return false
	}

	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return false
	}

	req := pagination.PaginationRequest{
		Limit: 10,
	}

	switch direction {
	case "before":
		req.Before = reference
	case "after":
		req.After = reference
	default:
		return false
	}

	relKeys, _, err := ti.ki.ExecuteKeyQuery(userThreadPrefix, req)
	if err != nil {
		return false
	}

	threadKeys := make([]string, 0, len(relKeys))
	for _, relKey := range relKeys {
		if parsed, err := keys.ParseUserOwnsThread(relKey); err == nil {
			threadKeys = append(threadKeys, parsed.ThreadKey)
		}
	}

	deleteMarkers := ti.getDeleteMarkersWithFallback(threadKeys)

	for _, threadKey := range threadKeys {
		if !deleteMarkers[threadKey] {
			if len(boundary) > 0 {
				switch direction {
				case "before":
					if threadKey > boundary[0] {
						return true
					}
				case "after":
					if threadKey < boundary[0] {
						return true
					}
				}
			} else {
				return true
			}
		}
	}

	return false
}

func (ti *ThreadIterator) checkHasNewerThreads(userID, newestKey string) (bool, error) {
	return ti.checkHasThreads(userID, newestKey, "before"), nil
}

func (ti *ThreadIterator) checkHasOlderThreads(userID, oldestKey string) (bool, error) {
	return ti.checkHasThreads(userID, oldestKey, "after"), nil
}

func (ti *ThreadIterator) getDeleteMarkersWithFallback(threadKeys []string) map[string]bool {
	deleteMarkers := make(map[string]bool)

	if len(threadKeys) == 0 {
		return deleteMarkers
	}

	deleteMarkerKeys := make([]string, 0, len(threadKeys))
	keyToOriginalMap := make(map[string]string)

	for _, threadKey := range threadKeys {
		deleteMarkerKey := keys.GenSoftDeleteMarkerKey(threadKey)
		deleteMarkerKeys = append(deleteMarkerKeys, deleteMarkerKey)
		keyToOriginalMap[deleteMarkerKey] = threadKey
	}

	for _, deleteMarkerKey := range deleteMarkerKeys {
		_, err := indexdb.GetKey(deleteMarkerKey)
		if err != nil {
			if indexdb.IsNotFound(err) {
				deleteMarkers[keyToOriginalMap[deleteMarkerKey]] = false
			} else {
				deleteMarkers[keyToOriginalMap[deleteMarkerKey]] = false
			}
		} else {
			deleteMarkers[keyToOriginalMap[deleteMarkerKey]] = true
		}
	}

	return deleteMarkers
}
