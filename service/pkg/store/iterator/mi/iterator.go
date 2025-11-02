package mi

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/pagination"
)

// MessageIterator handles message-specific pagination and counting
type MessageIterator struct {
	db *pebble.DB
}

// NewMessageIterator creates a new message iterator
func NewMessageIterator(db *pebble.DB) *MessageIterator {
	return &MessageIterator{db: db}
}

// GetMessageCount gets the total message count for a thread using the index system
func (mi *MessageIterator) GetMessageCount(threadKey string) (int, error) {
	// Use the existing index system which properly tracks message counts
	indexes, err := indexdb.GetThreadMessageIndexData(threadKey)
	if err != nil {
		// If index doesn't exist, fall back to counting
		return mi.countMessagesManually(threadKey)
	}

	// The End field in ThreadMessageIndexes is the total message count
	return int(indexes.End), nil
}

// countMessagesManually counts messages by iterating through message keys
func (mi *MessageIterator) countMessagesManually(threadKey string) (int, error) {
	// Generate message key prefix for this thread
	messagePrefix := fmt.Sprintf("t:%s:m:", threadKey)

	iter, err := mi.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(messagePrefix),
		UpperBound: nextPrefix([]byte(messagePrefix)),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	count := 0
	for valid := iter.First(); valid; valid = iter.Next() {
		count++
	}

	return count, nil
}

// ExecuteMessageQuery executes message pagination for a specific thread
func (mi *MessageIterator) ExecuteMessageQuery(threadKey string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	// Generate message key prefix for this thread
	messagePrefix := fmt.Sprintf("t:%s:m:", threadKey)

	// Create iterator with bounds
	iter, err := mi.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(messagePrefix),
		UpperBound: nextPrefix([]byte(messagePrefix)),
	})
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	var messageKeys []string
	var response pagination.PaginationResponse

	// Handle different query types exactly as specified
	switch {
	case req.Before != "" && req.After != "":
		// Both before and after - execute two distinct methods and merge
		beforeKeys, hasMoreBefore, err := mi.fetchBefore(iter, req.Before, req.Limit/2)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, hasMoreAfter, err := mi.fetchAfter(iter, req.After, req.Limit-len(beforeKeys))
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		messageKeys = append(beforeKeys, afterKeys...)
		response = pagination.PaginationResponse{
			HasBefore: hasMoreBefore,
			HasAfter:  hasMoreAfter,
			OrderBy:   req.OrderBy,
			Count:     len(messageKeys),
		}

	case req.Anchor != "":
		// Anchor - execute two distinct methods with anchor and merge with anchor in middle
		halfLimit := req.Limit / 2
		beforeKeys, hasMoreBefore, err := mi.fetchBefore(iter, req.Anchor, halfLimit)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, hasMoreAfter, err := mi.fetchAfter(iter, req.Anchor, req.Limit-len(beforeKeys))
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		// Merge: before + anchor + after
		messageKeys = append(append(beforeKeys, req.Anchor), afterKeys...)
		response = pagination.PaginationResponse{
			HasBefore: hasMoreBefore,
			HasAfter:  hasMoreAfter,
			OrderBy:   req.OrderBy,
			Count:     len(messageKeys),
		}

		// Set anchors for anchor case
		if len(messageKeys) > 0 {
			response.StartAnchor = messageKeys[0]
			response.EndAnchor = messageKeys[len(messageKeys)-1]
		}

	case req.Before != "":
		// Before only - seek to before key and get keys backwards
		messageKeys, response.HasBefore, err = mi.fetchBefore(iter, req.Before, req.Limit)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasAfter, _ = mi.checkHasAfter(iter, req.Before)
		response.OrderBy = req.OrderBy
		response.Count = len(messageKeys)

	case req.After != "":
		// After only - seek to after key and get keys forwards
		messageKeys, response.HasAfter, err = mi.fetchAfter(iter, req.After, req.Limit)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasBefore, _ = mi.checkHasBefore(iter, req.After)
		response.OrderBy = req.OrderBy
		response.Count = len(messageKeys)

	default:
		// No before/after/anchor - start from bottom and get keys backwards to limit
		messageKeys, response, err = mi.fetchInitialLoad(iter, req)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
	}

	// Get total count of all messages for this thread
	total, err := mi.GetMessageCount(threadKey)
	if err != nil {
		// Log error but don't fail the request
		total = 0
	}
	response.Total = total

	// Update response anchors
	if len(messageKeys) > 0 {
		response.StartAnchor = messageKeys[0]
		response.EndAnchor = messageKeys[len(messageKeys)-1]
	}

	return messageKeys, response, nil
}

// fetchBefore seeks to before key and gets keys backwards to limit
func (mi *MessageIterator) fetchBefore(iter *pebble.Iterator, reference string, limit int) ([]string, bool, error) {
	var items []string
	referenceKey := []byte(reference)

	// Seek to reference point and go backward
	valid := iter.SeekLT(referenceKey)

	// Collect items going backward
	for valid && len(items) < limit {
		key := string(iter.Key())
		items = append(items, key)
		valid = iter.Prev()
	}

	// Check for more items
	hasMore := valid

	// Reverse items to maintain correct order
	if len(items) > 0 {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}

	return items, hasMore, nil
}

// fetchAfter seeks to after key and gets keys forwards to limit
func (mi *MessageIterator) fetchAfter(iter *pebble.Iterator, reference string, limit int) ([]string, bool, error) {
	var items []string
	referenceKey := []byte(reference)

	// Seek to reference point and go forward
	valid := iter.SeekGE(referenceKey)
	if valid && string(iter.Key()) == reference {
		valid = iter.Next() // Skip the reference itself
	}

	// Collect items going forward
	for valid && len(items) < limit {
		key := string(iter.Key())
		items = append(items, key)
		valid = iter.Next()
	}

	// Check for more items
	hasMore := valid

	return items, hasMore, nil
}

// fetchInitialLoad starts from bottom and gets keys backwards to limit
func (mi *MessageIterator) fetchInitialLoad(iter *pebble.Iterator, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	var items []string

	// Start from the bottom (newest first)
	valid := iter.Last()

	// Collect items going backward to limit
	for valid && len(items) < req.Limit {
		key := string(iter.Key())
		items = append(items, key)
		valid = iter.Prev()
	}

	// Check for more items
	hasMore := valid

	response := pagination.PaginationResponse{
		HasBefore: hasMore,
		HasAfter:  false, // No items after when starting from bottom
		OrderBy:   req.OrderBy,
		Count:     len(items),
	}

	// Set anchors if we have items
	if len(items) > 0 {
		response.StartAnchor = items[0]
		response.EndAnchor = items[len(items)-1]
	}

	return items, response, nil
}

// checkHasBefore checks if there are items before the reference point
func (mi *MessageIterator) checkHasBefore(iter *pebble.Iterator, reference string) (bool, error) {
	referenceKey := []byte(reference)
	valid := iter.SeekLT(referenceKey)
	return valid, nil
}

// checkHasAfter checks if there are items after the reference point
func (mi *MessageIterator) checkHasAfter(iter *pebble.Iterator, reference string) (bool, error) {
	referenceKey := []byte(reference)
	valid := iter.SeekGE(referenceKey)
	if valid && string(iter.Key()) == reference {
		valid = iter.Next() // Skip the reference itself
	}
	return valid, nil
}

// nextPrefix computes the next lexicographical key after a given prefix
func nextPrefix(prefix []byte) []byte {
	out := make([]byte, len(prefix))
	copy(out, prefix)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] < 0xFF {
			out[i]++
			return out[:i+1]
		}
	}
	return nil // no upper bound if all 0xFF
}
