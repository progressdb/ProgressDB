package ti

import (
	"fmt"

	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
)

// ThreadIterator handles thread-specific pagination
type ThreadIterator struct {
	db *pebble.DB
}

// NewThreadIterator creates a new thread iterator
func NewThreadIterator(db *pebble.DB) *ThreadIterator {
	return &ThreadIterator{db: db}
}

// ExecuteThreadQuery executes a thread pagination query for a specific user
func (ti *ThreadIterator) ExecuteThreadQuery(userID string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	// Generate user thread relationship prefix
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to generate user thread prefix: %w", err)
	}

	// Create iterator with bounds
	iter, err := ti.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(userThreadPrefix),
		UpperBound: nextPrefix([]byte(userThreadPrefix)),
	})
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	var relationshipKeys []string
	var response pagination.PaginationResponse

	// Handle different query types exactly as specified
	switch {
	case req.Before != "" && req.After != "":
		// Both before and after - execute two distinct methods and merge
		beforeKeys, hasMoreBefore, err := ti.fetchBefore(iter, req.Before, req.Limit/2)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, hasMoreAfter, err := ti.fetchAfter(iter, req.After, req.Limit-len(beforeKeys))
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		relationshipKeys = append(beforeKeys, afterKeys...)
		response = pagination.PaginationResponse{
			HasBefore: hasMoreBefore,
			HasAfter:  hasMoreAfter,
			OrderBy:   req.OrderBy,
			Count:     len(relationshipKeys),
		}

	case req.Anchor != "":
		// Anchor - execute two distinct methods with anchor and merge with anchor in middle
		halfLimit := req.Limit / 2
		beforeKeys, hasMoreBefore, err := ti.fetchBefore(iter, req.Anchor, halfLimit)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, hasMoreAfter, err := ti.fetchAfter(iter, req.Anchor, req.Limit-len(beforeKeys))
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		// Merge: before + anchor + after
		relationshipKeys = append(append(beforeKeys, req.Anchor), afterKeys...)
		response = pagination.PaginationResponse{
			HasBefore: hasMoreBefore,
			HasAfter:  hasMoreAfter,
			OrderBy:   req.OrderBy,
			Count:     len(relationshipKeys),
		}

		// Set anchors for anchor case
		if len(relationshipKeys) > 0 {
			response.StartAnchor = relationshipKeys[0]
			response.EndAnchor = relationshipKeys[len(relationshipKeys)-1]
		}

	case req.Before != "":
		// Before only - seek to before key and get keys backwards
		relationshipKeys, response.HasBefore, err = ti.fetchBefore(iter, req.Before, req.Limit)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasAfter, _ = ti.checkHasAfter(iter, req.Before)
		response.OrderBy = req.OrderBy
		response.Count = len(relationshipKeys)

	case req.After != "":
		// After only - seek to after key and get keys forwards
		relationshipKeys, response.HasAfter, err = ti.fetchAfter(iter, req.After, req.Limit)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasBefore, _ = ti.checkHasBefore(iter, req.After)
		response.OrderBy = req.OrderBy
		response.Count = len(relationshipKeys)

	default:
		// No before/after/anchor - start from bottom and get keys backwards to limit
		relationshipKeys, response, err = ti.fetchInitialLoad(iter, req)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
	}

	// Get total count of all threads for this user
	total, err := ti.getTotalThreadCount(userID)
	if err != nil {
		// Log error but don't fail the request
		total = 0
	}
	response.Total = total

	// Convert relationship keys to thread keys
	threadKeys := make([]string, 0, len(relationshipKeys))
	for _, relKey := range relationshipKeys {
		parsed, err := keys.ParseUserOwnsThread(relKey)
		if err != nil {
			continue // Skip invalid keys
		}
		threadKeys = append(threadKeys, parsed.ThreadKey)
	}

	// Update response anchors to use thread keys
	if len(threadKeys) > 0 {
		response.StartAnchor = threadKeys[0]
		response.EndAnchor = threadKeys[len(threadKeys)-1]
	}

	return threadKeys, response, nil
}

// fetchBefore seeks to before key and gets keys backwards to limit
func (ti *ThreadIterator) fetchBefore(iter *pebble.Iterator, reference string, limit int) ([]string, bool, error) {
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
func (ti *ThreadIterator) fetchAfter(iter *pebble.Iterator, reference string, limit int) ([]string, bool, error) {
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
func (ti *ThreadIterator) fetchInitialLoad(iter *pebble.Iterator, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
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
func (ti *ThreadIterator) checkHasBefore(iter *pebble.Iterator, reference string) (bool, error) {
	referenceKey := []byte(reference)
	valid := iter.SeekLT(referenceKey)
	return valid, nil
}

// checkHasAfter checks if there are items after the reference point
func (ti *ThreadIterator) checkHasAfter(iter *pebble.Iterator, reference string) (bool, error) {
	referenceKey := []byte(reference)
	valid := iter.SeekGE(referenceKey)
	if valid && string(iter.Key()) == reference {
		valid = iter.Next() // Skip the reference itself
	}
	return valid, nil
}

// getTotalThreadCount counts all threads for a user
func (ti *ThreadIterator) getTotalThreadCount(userID string) (int, error) {
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return 0, err
	}

	iter, err := ti.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(userThreadPrefix),
		UpperBound: nextPrefix([]byte(userThreadPrefix)),
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	count := 0
	for valid := iter.First(); valid; valid = iter.Next() {
		count++
	}

	return count, nil
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
