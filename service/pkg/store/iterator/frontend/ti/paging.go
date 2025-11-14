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

		// Swap the checks: Check after for first, before for last
		response.HasAfter = pm.hasThreadsAfterAnchor(firstThread, userID)
		response.HasBefore = pm.hasThreadsBeforeAnchor(lastThread, userID)

		// Set anchor points for next navigation
		if len(threads) > 0 {
			response.BeforeAnchor = firstThread.Key
			response.AfterAnchor = lastThread.Key
		}

	case req.Before != "":
		// Before query: we're at the newest, check for older threads than reference
		response.HasBefore = false // We're showing the newest threads

		userThreadPrefix, _ := keys.GenUserThreadRelPrefix(userID)
		refRelKey := req.Before
		if !strings.HasPrefix(req.Before, "rel:u:") {
			refRelKey = fmt.Sprintf(keys.RelUserOwnsThread, userID, strings.TrimPrefix(req.Before, "t:"))
		}

		// Check for threads older than reference
		response.HasAfter = pm.keys.checkHasKeysAfter(userThreadPrefix, refRelKey)

		// Set anchor for getting older threads
		if len(threads) > 0 {
			response.AfterAnchor = threads[len(threads)-1].Key
		}

	case req.After != "":
		// After query: check for newer threads than reference, and older than last result
		userThreadPrefix, _ := keys.GenUserThreadRelPrefix(userID)
		refRelKey := req.After
		if !strings.HasPrefix(req.After, "rel:u:") {
			// Convert thread key to relationship key
			refRelKey = fmt.Sprintf(keys.RelUserOwnsThread, userID, strings.TrimPrefix(req.After, "t:"))
		}

		// Check for threads newer than reference
		response.HasBefore = pm.keys.checkHasKeysBefore(userThreadPrefix, refRelKey)

		// Check for threads older than last result
		lastRelKey := ""
		if len(threads) > 0 {
			lastRelKey = fmt.Sprintf(keys.RelUserOwnsThread, userID, strings.TrimPrefix(threads[len(threads)-1].Key, "t:"))
		}
		response.HasAfter = pm.keys.checkHasKeysAfter(userThreadPrefix, lastRelKey)

		// Set anchor for getting newer threads
		if len(threads) > 0 {
			response.BeforeAnchor = threads[0].Key
		}

	default:
		// Initial load: newest threads first
		response.HasBefore = false               // We're at the newest
		response.HasAfter = len(threads) < total // More older threads exist

		// Set anchors for pagination
		if len(threads) > 0 {
			response.BeforeAnchor = threads[len(threads)-1].Key // For getting older
			response.AfterAnchor = threads[0].Key               // For getting newer (though unlikely)
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
