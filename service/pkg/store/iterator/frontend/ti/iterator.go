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
	relationshipKeys, kiResponse, err := keyIter.ExecuteKeyQuery(userThreadPrefix, req)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	// Filter out deleted threads BEFORE setting pagination metadata
	filteredRelKeys := make([]string, 0, len(relationshipKeys))
	for _, relKey := range relationshipKeys {
		parsed, err := keys.ParseUserOwnsThread(relKey)
		if err != nil {
			continue
		}

		// Check if thread is deleted by fetching thread data
		threadDeleted, err := ti.isThreadDeleted(parsed.ThreadKey)
		if err != nil {
			// If we can't determine deletion status, include the thread (fail-safe)
			filteredRelKeys = append(filteredRelKeys, relKey)
			continue
		}

		// Only include non-deleted threads
		if !threadDeleted {
			filteredRelKeys = append(filteredRelKeys, relKey)
		}
	}

	// Get total thread count (excluding deleted)
	total, err := ti.getTotalThreadCount(userID)
	if err != nil {
		total = 0
	}

	// Create new response with correct counts and anchors based on filtered data
	response := pagination.PaginationResponse{
		HasBefore: kiResponse.HasBefore,
		HasAfter:  kiResponse.HasAfter,
		Count:     len(filteredRelKeys),
		Total:     total,
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
	threadKeys := make([]string, 0, len(sortedKeys))
	for _, relKey := range sortedKeys {
		parsed, err := keys.ParseUserOwnsThread(relKey)
		if err != nil {
			continue
		}
		threadKeys = append(threadKeys, parsed.ThreadKey)
	}

	return threadKeys, response, nil
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

	// Filter out deleted threads
	count := 0
	for _, relKey := range relationshipKeys {
		parsed, err := keys.ParseUserOwnsThread(relKey)
		if err != nil {
			continue
		}

		threadDeleted, err := ti.isThreadDeleted(parsed.ThreadKey)
		if err != nil {
			// If we can't determine deletion status, count it (fail-safe)
			count++
			continue
		}

		// Only count non-deleted threads
		if !threadDeleted {
			count++
		}
	}

	return count, nil
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
