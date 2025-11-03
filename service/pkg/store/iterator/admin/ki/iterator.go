package ki

import (
	"fmt"

	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
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
	// Add logs for input
	fmt.Printf("[KeyIterator] ExecuteKeyQuery: prefix=%q, req=%+v\n", prefix, req)

	// Handle empty prefix - scan entire database
	var iter *pebble.Iterator
	var err error

	if prefix == "" {
		fmt.Printf("[KeyIterator] Empty prefix - scanning entire database\n")
		iter, err = ki.db.NewIter(&pebble.IterOptions{})
	} else {
		// Debug bounds for non-empty prefix
		lowerBound := []byte(prefix)
		upperBound := nextPrefix([]byte(prefix))
		fmt.Printf("[KeyIterator] Iterator bounds: lower=%q, upper=%q\n", string(lowerBound), string(upperBound))
		iter, err = ki.db.NewIter(&pebble.IterOptions{
			LowerBound: lowerBound,
			UpperBound: upperBound,
		})
	}
	if err != nil {
		fmt.Printf("[KeyIterator] Error creating iterator: %v\n", err)
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	var keys []string
	var response pagination.PaginationResponse

	// Handle different query types exactly as specified
	switch {
	case req.Anchor != "":
		fmt.Printf("[KeyIterator] Using anchor mode. Anchor=%q, Limit=%d\n", req.Anchor, req.Limit)
		beforeKeys, hasMoreBefore, err := ki.fetchBefore(iter, req.Anchor, req.Limit/2)
		if err != nil {
			fmt.Printf("[KeyIterator] fetchBefore error (anchor): %v\n", err)
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, hasMoreAfter, err := ki.fetchAfter(iter, req.Anchor, req.Limit-len(beforeKeys))
		if err != nil {
			fmt.Printf("[KeyIterator] fetchAfter error (anchor): %v\n", err)
			return nil, pagination.PaginationResponse{}, err
		}
		keys = append(beforeKeys, req.Anchor)
		keys = append(keys, afterKeys...)
		response = pagination.PaginationResponse{
			HasBefore: hasMoreBefore,
			HasAfter:  hasMoreAfter,
			OrderBy:   req.OrderBy,
			Count:     len(keys),
			Total:     ki.getTotalCount(iter),
		}
		fmt.Printf("[KeyIterator] Anchor results: beforeKeys=%d, afterKeys=%d, totalKeys=%d\n", len(beforeKeys), len(afterKeys), len(keys))

	case req.Before != "" && req.After != "":
		fmt.Printf("[KeyIterator] Using before+after mode. Before=%q, After=%q, Limit=%d\n", req.Before, req.After, req.Limit)
		beforeKeys, hasMoreBefore, err := ki.fetchBefore(iter, req.Before, req.Limit/2)
		if err != nil {
			fmt.Printf("[KeyIterator] fetchBefore error (before+after): %v\n", err)
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, hasMoreAfter, err := ki.fetchAfter(iter, req.After, req.Limit-len(beforeKeys))
		if err != nil {
			fmt.Printf("[KeyIterator] fetchAfter error (before+after): %v\n", err)
			return nil, pagination.PaginationResponse{}, err
		}
		keys = append(beforeKeys, afterKeys...)
		response = pagination.PaginationResponse{
			HasBefore: hasMoreBefore,
			HasAfter:  hasMoreAfter,
			OrderBy:   req.OrderBy,
			Count:     len(keys),
			Total:     ki.getTotalCount(iter),
		}
		fmt.Printf("[KeyIterator] Before+After results: beforeKeys=%d, afterKeys=%d, totalKeys=%d\n", len(beforeKeys), len(afterKeys), len(keys))

	case req.Before != "":
		fmt.Printf("[KeyIterator] Using before mode. Before=%q, Limit=%d\n", req.Before, req.Limit)
		keys, response.HasBefore, err = ki.fetchBefore(iter, req.Before, req.Limit)
		if err != nil {
			fmt.Printf("[KeyIterator] fetchBefore error: %v\n", err)
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasAfter, _ = ki.checkHasAfter(iter, req.Before)
		response.OrderBy = req.OrderBy
		response.Count = len(keys)
		response.Total = ki.getTotalCount(iter)
		fmt.Printf("[KeyIterator] Before mode keys=%d, HasBefore=%v, HasAfter=%v\n", len(keys), response.HasBefore, response.HasAfter)

	case req.After != "":
		fmt.Printf("[KeyIterator] Using after mode. After=%q, Limit=%d\n", req.After, req.Limit)
		keys, response.HasAfter, err = ki.fetchAfter(iter, req.After, req.Limit)
		if err != nil {
			fmt.Printf("[KeyIterator] fetchAfter error: %v\n", err)
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasBefore, _ = ki.checkHasBefore(iter, req.After)
		response.OrderBy = req.OrderBy
		response.Count = len(keys)
		response.Total = ki.getTotalCount(iter)
		fmt.Printf("[KeyIterator] After mode keys=%d, HasBefore=%v, HasAfter=%v\n", len(keys), response.HasBefore, response.HasAfter)

	default:
		fmt.Printf("[KeyIterator] Using default initial load. Limit=%d\n", req.Limit)
		keys, response, err = ki.fetchInitialLoad(iter, req)
		if err != nil {
			fmt.Printf("[KeyIterator] fetchInitialLoad error: %v\n", err)
			return nil, pagination.PaginationResponse{}, err
		}
		fmt.Printf("[KeyIterator] Initial load keys=%d\n", len(keys))
	}

	// Set anchors if we have keys
	if len(keys) > 0 {
		// Always set StartAnchor to the first element and EndAnchor to the last element in the keys array
		response.StartAnchor = keys[0]
		response.EndAnchor = keys[len(keys)-1]
		fmt.Printf("[KeyIterator] Set anchors: StartAnchor=%q, EndAnchor=%q\n", response.StartAnchor, response.EndAnchor)
	} else {
		fmt.Printf("[KeyIterator] No keys found for this query.\n")
	}

	fmt.Printf("[KeyIterator] Returning %d keys, Response=%+v\n", len(keys), response)
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
	fmt.Printf("[KeyIterator] iter.Last() valid=%v\n", valid)

	if valid {
		fmt.Printf("[KeyIterator] First key from bottom: %q\n", string(iter.Key()))
	}

	// Collect items going backward to limit
	for valid && len(items) < req.Limit {
		key := string(iter.Key())
		fmt.Printf("[KeyIterator] Adding key: %q\n", key)
		items = append(items, key)
		valid = iter.Prev()
	}

	fmt.Printf("[KeyIterator] Loop ended. valid=%v, items=%d\n", valid, len(items))

	// Check for more items
	hasMore := valid

	response := pagination.PaginationResponse{
		HasBefore: hasMore,
		HasAfter:  false, // No items after when starting from bottom
		OrderBy:   req.OrderBy,
		Count:     len(items),
		Total:     ki.getTotalCount(iter),
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

// getTotalCount returns the total count of keys within the iterator bounds
func (ki *KeyIterator) getTotalCount(iter *pebble.Iterator) int {
	count := 0
	// Reset iterator to start
	valid := iter.First()
	fmt.Printf("[KeyIterator] getTotalCount: iter.First() valid=%v\n", valid)

	for valid {
		if count < 5 { // Only log first 5 keys to avoid spam
			fmt.Printf("[KeyIterator] getTotalCount key[%d]: %q\n", count, string(iter.Key()))
		}
		count++
		valid = iter.Next()
	}
	fmt.Printf("[KeyIterator] getTotalCount final count: %d\n", count)
	return count
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
	return append(next, 0x00)
}
