package ki

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/pagination"
)

// KeyIterator handles pure key-based pagination for admin operations
type KeyIterator struct {
	db *pebble.DB
}

// NewKeyIterator creates a new key iterator
func NewKeyIterator(db *pebble.DB) *KeyIterator {
	return &KeyIterator{db: db}
}

// ExecuteKeyQuery executes a key pagination query with given prefix
func (ki *KeyIterator) ExecuteKeyQuery(prefix string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	// Create iterator with bounds
	iter, err := ki.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: nextPrefix([]byte(prefix)),
	})
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	var keys []string
	var response pagination.PaginationResponse

	// Handle different query types exactly as specified
	switch {
	case req.Before != "" && req.After != "":
		// Both before and after - execute two distinct methods and merge
		beforeKeys, hasMoreBefore, err := ki.fetchBefore(iter, req.Before, req.Limit/2)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, hasMoreAfter, err := ki.fetchAfter(iter, req.After, req.Limit-len(beforeKeys))
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		// Merge: before + after
		keys = append(beforeKeys, afterKeys...)
		response = pagination.PaginationResponse{
			HasBefore: hasMoreBefore,
			HasAfter:  hasMoreAfter,
			OrderBy:   req.OrderBy,
			Count:     len(keys),
		}

	case req.Before != "":
		// Before only - seek to before key and get keys backwards
		keys, response.HasBefore, err = ki.fetchBefore(iter, req.Before, req.Limit)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasAfter, _ = ki.checkHasAfter(iter, req.Before)
		response.OrderBy = req.OrderBy
		response.Count = len(keys)

	case req.After != "":
		// After only - seek to after key and get keys forwards
		keys, response.HasAfter, err = ki.fetchAfter(iter, req.After, req.Limit)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasBefore, _ = ki.checkHasBefore(iter, req.After)
		response.OrderBy = req.OrderBy
		response.Count = len(keys)

	default:
		// No before/after/anchor - start from bottom and get keys backwards to limit
		keys, response, err = ki.fetchInitialLoad(iter, req)
		if err != nil {
			return nil, pagination.PaginationResponse{}, err
		}
	}

	// Set anchors if we have keys
	if len(keys) > 0 {
		response.StartAnchor = keys[0]
		response.EndAnchor = keys[len(keys)-1]
	}

	return keys, response, nil
}

// fetchBefore seeks to before key and gets keys backwards to limit
func (ki *KeyIterator) fetchBefore(iter *pebble.Iterator, reference string, limit int) ([]string, bool, error) {
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
	hasMore := valid && len(items) >= limit

	return items, hasMore, iter.Error()
}

// fetchAfter seeks to after key and gets keys forwards to limit
func (ki *KeyIterator) fetchAfter(iter *pebble.Iterator, reference string, limit int) ([]string, bool, error) {
	var items []string

	// Seek to reference point and go forward
	valid := iter.SeekGE([]byte(reference))
	// Skip the reference key itself to get "after"
	if valid && string(iter.Key()) == reference {
		valid = iter.Next()
	}

	// Collect items going forward
	for valid && len(items) < limit {
		key := string(iter.Key())
		items = append(items, key)
		valid = iter.Next()
	}

	// Check for more items
	hasMore := valid && len(items) >= limit

	return items, hasMore, iter.Error()
}

// fetchInitialLoad starts from bottom and gets keys backwards to limit
func (ki *KeyIterator) fetchInitialLoad(iter *pebble.Iterator, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
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

// checkHasBefore checks if there are items before reference point
func (ki *KeyIterator) checkHasBefore(iter *pebble.Iterator, reference string) (bool, error) {
	referenceKey := []byte(reference)

	// Seek to reference point and check if there's a previous item
	valid := iter.SeekLT(referenceKey)

	return valid, iter.Error()
}

// checkHasAfter checks if there are items after reference point
func (ki *KeyIterator) checkHasAfter(iter *pebble.Iterator, reference string) (bool, error) {
	// Seek to reference point and check if there's a next item
	valid := iter.SeekGE([]byte(reference))
	// Skip the reference key itself
	if valid && string(iter.Key()) == reference {
		valid = iter.Next()
	}

	return valid, iter.Error()
}

// nextPrefix returns the next prefix after the given prefix for range scanning
func nextPrefix(prefix []byte) []byte {
	next := make([]byte, len(prefix))
	copy(next, prefix)
	for i := len(next) - 1; i >= 0; i-- {
		if next[i] < 0xff {
			next[i]++
			return next[:i+1]
		}
	}
	return prefix // overflow, return original
}
