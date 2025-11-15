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

	// Set pagination flags based on query type
	switch {
	case req.Anchor != "":
		// Anchor query: check if there are messages before/after the current window

		firstMessage := messages[0]
		lastMessage := messages[len(messages)-1]

		// Special case: if we have all messages, no navigation available
		if len(messages) == total {
			response.HasBefore = false
			response.HasAfter = false
		} else {
			// Calculate flags FIRST - check navigation relative to window boundaries
			response.HasAfter = pm.hasMessagesAfterAnchor(lastMessage, threadKey)
			response.HasBefore = pm.hasMessagesBeforeAnchor(firstMessage, threadKey)
		}

		// Set anchor points - for anchor queries, before_anchor is oldest, after_anchor is newest
		if len(messages) > 0 {
			response.BeforeAnchor = firstMessage.Key // oldest message in window (for navigating to older pages)
			response.AfterAnchor = lastMessage.Key   // newest message in window (for navigating to newer pages)
		}

	case req.Before != "":
		// Before query: get messages older than reference point
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
		// Initial load: oldest messages first (for chat display)
		response.HasBefore = len(messages) < total // There are older messages
		response.HasAfter = false                  // We're at the newest

		// Set anchors based on navigation availability (like TI)
		if len(messages) > 0 {
			if response.HasBefore {
				response.BeforeAnchor = messages[0].Key
			} else {
				response.BeforeAnchor = messages[len(messages)-1].Key
			}

			if response.HasAfter {
				response.AfterAnchor = messages[len(messages)-1].Key
			} else {
				response.AfterAnchor = messages[0].Key
			}
		}
	}

	return response
}

func (pm *PageManager) hasMessagesAfterAnchor(message models.Message, threadKey string) bool {
	messagePrefix, _ := keys.GenAllThreadMessagesPrefix(threadKey)
	return pm.keys.checkHasKeysAfter(messagePrefix, message.Key)
}

func (pm *PageManager) hasMessagesBeforeAnchor(message models.Message, threadKey string) bool {
	messagePrefix, _ := keys.GenAllThreadMessagesPrefix(threadKey)
	return pm.keys.checkHasKeysBefore(messagePrefix, message.Key)
}
