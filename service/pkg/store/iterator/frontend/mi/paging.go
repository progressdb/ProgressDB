package mi

import (
	"progressdb/pkg/models"
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
	messages []models.Message,
	total int,
	req pagination.PaginationRequest,
	threadKey string,
) pagination.PaginationResponse {

	response := pagination.PaginationResponse{
		Count: len(messages),
		Total: total,
	}

	if len(messages) == 0 {
		response.HasBefore = false
		response.HasAfter = false
		return response
	}

	switch {
	case req.Anchor != "":

		firstMessage := messages[0]
		lastMessage := messages[len(messages)-1]

		if len(messages) == total {
			response.HasBefore = false
			response.HasAfter = false
		} else {

			messagePrefix, _ := keys.GenAllThreadMessagesPrefix(threadKey)
			response.HasAfter = pm.keys.checkHasKeysAfter(messagePrefix, lastMessage.Key)
			response.HasBefore = pm.keys.checkHasKeysBefore(messagePrefix, firstMessage.Key)
		}

		if len(messages) > 0 {
			response.BeforeAnchor = firstMessage.Key
			response.AfterAnchor = lastMessage.Key
		}

	case req.Before != "":

		messagePrefix, _ := keys.GenAllThreadMessagesPrefix(threadKey)

		// Calculate flags relative to window boundaries (like TI)
		if len(messages) > 0 {
			// For before queries, check if there are messages newer than newest message in current window
			newestMessageKey := messages[len(messages)-1].Key
			response.HasAfter = pm.keys.checkHasKeysAfter(messagePrefix, newestMessageKey)

			// Check if there are messages older than oldest message in current window
			oldestMessageKey := messages[0].Key
			response.HasBefore = pm.keys.checkHasKeysBefore(messagePrefix, oldestMessageKey)
		} else {
			// No messages returned, use original reference for consistency
			response.HasBefore = pm.keys.checkHasKeysBefore(messagePrefix, req.Before)
			response.HasAfter = pm.keys.checkHasKeysAfter(messagePrefix, req.Before)
		}

		// Set anchors for before query: before_anchor is oldest, after_anchor is newest (like TI)
		if len(messages) > 0 {
			response.BeforeAnchor = messages[0].Key              // oldest message (for continuing backwards)
			response.AfterAnchor = messages[len(messages)-1].Key // newest message (for going forwards)
		}

	case req.After != "":
		// After query: get messages newer than reference point
		messagePrefix, _ := keys.GenAllThreadMessagesPrefix(threadKey)

		// Calculate flags relative to window boundaries (like TI)
		if len(messages) > 0 {
			// For after queries, check if there are messages older than oldest message in current window
			oldestMessageKey := messages[0].Key
			response.HasBefore = pm.keys.checkHasKeysBefore(messagePrefix, oldestMessageKey)

			// Check if there are messages newer than newest message in current window
			newestMessageKey := messages[len(messages)-1].Key
			response.HasAfter = pm.keys.checkHasKeysAfter(messagePrefix, newestMessageKey)
		} else {
			// No messages returned, use original reference for consistency
			response.HasBefore = pm.keys.checkHasKeysBefore(messagePrefix, req.After)
			response.HasAfter = pm.keys.checkHasKeysAfter(messagePrefix, req.After)
		}

		// Set anchors for after query: before_anchor is oldest, after_anchor is newest
		if len(messages) > 0 {
			response.BeforeAnchor = messages[0].Key              // oldest message (for continuing backwards)
			response.AfterAnchor = messages[len(messages)-1].Key // newest message (for continuing forwards)
		}

	default:
		response.HasBefore = len(messages) < total
		response.HasAfter = false

		if len(messages) > 0 {
			response.BeforeAnchor = messages[0].Key
			response.AfterAnchor = messages[len(messages)-1].Key
		}
	}

	return response
}
