package ti

import (
	"fmt"
	"strings"

	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

type PageManager struct {
	keys *KeyManager
}

func NewPageManager(keys *KeyManager) *PageManager {
	return &PageManager{keys: keys}
}

func (pm *PageManager) CalculatePagination(
	threads []models.Thread,
	total int,
	req pagination.PaginationRequest,
	userID string,
) pagination.PaginationResponse {

	response := pagination.PaginationResponse{
		Count: len(threads),
		Total: total,
	}

	if len(threads) == 0 {
		response.HasBefore = false
		response.HasAfter = false
		return response
	}

	// Set pagination flags based on query type
	switch {
	case req.Anchor != "":
		// Anchor query: check if there are threads before/after the current window

		firstThread := threads[0]
		lastThread := threads[len(threads)-1]

		// Calculate flags FIRST - check navigation relative to window boundaries
		response.HasAfter = pm.hasThreadsAfterAnchor(firstThread, userID)
		response.HasBefore = pm.hasThreadsBeforeAnchor(lastThread, userID)

		// Set anchor points - for anchor queries, before_anchor is oldest, after_anchor is newest
		if len(threads) > 0 {
			response.BeforeAnchor = lastThread.Key // oldest thread in window (for navigating to older pages)
			response.AfterAnchor = firstThread.Key // newest thread in window (for navigating to newer pages)
		}

	case req.Before != "":
		// Before query: get threads older than reference point
		userThreadPrefix, _ := keys.GenUserThreadRelPrefix(userID)

		// Convert reference to relationship key format for comparison
		originalRefRelKey := req.Before
		if !strings.HasPrefix(req.Before, "rel:u:") {
			originalRefRelKey = fmt.Sprintf(keys.RelUserOwnsThread, userID, strings.TrimPrefix(req.Before, "t:"))
		}

		// Calculate flags relative to window boundaries
		if len(threads) > 0 {
			// For before queries, check if there are threads newer than newest thread in current window
			newestThreadKey := threads[0].Key
			newestRelKey, err := keys.GenUserThreadRelPrefix(userID)
			if err == nil {
				newestRelKey += strings.TrimPrefix(newestThreadKey, "t:")
				response.HasAfter = pm.keys.checkHasKeysAfter(userThreadPrefix, newestRelKey)
			}

			// Check if there are threads older than oldest thread in current window
			oldestThreadKey := threads[len(threads)-1].Key
			oldestRelKey, err := keys.GenUserThreadRelPrefix(userID)
			if err == nil {
				oldestRelKey += strings.TrimPrefix(oldestThreadKey, "t:")
				response.HasBefore = pm.keys.checkHasKeysBefore(userThreadPrefix, oldestRelKey)
			}
		} else {
			// No threads returned, use original reference for consistency
			response.HasBefore = pm.keys.checkHasKeysBefore(userThreadPrefix, originalRefRelKey)
			response.HasAfter = pm.keys.checkHasKeysAfter(userThreadPrefix, originalRefRelKey)
		}

		// Set anchors for before query: before_anchor is oldest, after_anchor is newest
		if len(threads) > 0 {
			response.BeforeAnchor = threads[len(threads)-1].Key // oldest thread (for continuing backwards)
			response.AfterAnchor = threads[0].Key               // newest thread (for going forwards)
		}

	case req.After != "":
		// After query: get threads newer than reference point
		userThreadPrefix, _ := keys.GenUserThreadRelPrefix(userID)

		// Convert reference to relationship key format for comparison
		originalRefRelKey := req.After
		if !strings.HasPrefix(req.After, "rel:u:") {
			originalRefRelKey = fmt.Sprintf(keys.RelUserOwnsThread, userID, strings.TrimPrefix(req.After, "t:"))
		}

		// Calculate flags relative to window boundaries
		if len(threads) > 0 {
			// For after queries, check if there are threads older than oldest thread in current window
			oldestThreadKey := threads[len(threads)-1].Key
			oldestRelKey, err := keys.GenUserThreadRelPrefix(userID)
			if err == nil {
				oldestRelKey += strings.TrimPrefix(oldestThreadKey, "t:")
				response.HasBefore = pm.keys.checkHasKeysBefore(userThreadPrefix, oldestRelKey)
			}

			// Check if there are threads newer than newest thread in current window
			newestThreadKey := threads[0].Key
			newestRelKey, err := keys.GenUserThreadRelPrefix(userID)
			if err == nil {
				newestRelKey += strings.TrimPrefix(newestThreadKey, "t:")
				response.HasAfter = pm.keys.checkHasKeysAfter(userThreadPrefix, newestRelKey)
			}
		} else {
			// No threads returned, use original reference for consistency
			response.HasBefore = pm.keys.checkHasKeysBefore(userThreadPrefix, originalRefRelKey)
			response.HasAfter = pm.keys.checkHasKeysAfter(userThreadPrefix, originalRefRelKey)
		}

		// Set anchors for after query: before_anchor is newest, after_anchor is oldest
		if len(threads) > 0 {
			response.BeforeAnchor = threads[0].Key             // newest thread (for continuing forwards)
			response.AfterAnchor = threads[len(threads)-1].Key // oldest thread (for going backwards)
		}

	default:
		// Initial load: newest threads first
		response.HasBefore = false               // We're at the newest
		response.HasAfter = len(threads) < total // More older threads exist

		// Set anchors based on navigation availability
		if len(threads) > 0 {
			if response.HasBefore {
				response.BeforeAnchor = threads[0].Key
			} else {
				response.BeforeAnchor = threads[len(threads)-1].Key
			}

			if response.HasAfter {
				response.AfterAnchor = threads[len(threads)-1].Key
			} else {
				response.AfterAnchor = threads[0].Key
			}
		}
	}

	return response
}

func (pm *PageManager) hasThreadsBeforeAnchor(thread models.Thread, userID string) bool {
	userThreadPrefix, _ := keys.GenUserThreadRelPrefix(userID)
	relKey := fmt.Sprintf(keys.RelUserOwnsThread, userID, strings.TrimPrefix(thread.Key, "t:"))

	hasBefore := pm.keys.checkHasKeysBefore(userThreadPrefix, relKey)

	logger.Debug("[hasThreadsBeforeAnchor]", "userID", userID, "threadKey", thread.Key, "relKey", relKey, "hasBefore", hasBefore)
	return hasBefore
}

func (pm *PageManager) hasThreadsAfterAnchor(thread models.Thread, userID string) bool {
	userThreadPrefix, _ := keys.GenUserThreadRelPrefix(userID)
	relKey := fmt.Sprintf(keys.RelUserOwnsThread, userID, strings.TrimPrefix(thread.Key, "t:"))

	hasAfter := pm.keys.checkHasKeysAfter(userThreadPrefix, relKey)

	logger.Debug("[hasThreadsAfterAnchor]", "userID", userID, "threadKey", thread.Key, "relKey", relKey, "hasAfter", hasAfter)
	return hasAfter
}
