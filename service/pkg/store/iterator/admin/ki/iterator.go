package ki

import (
	"fmt"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
)

type KeyIterator struct {
	db *pebble.DB
}

func NewKeyIterator(db *pebble.DB) *KeyIterator {
	return &KeyIterator{db: db}
}

func (ki *KeyIterator) ExecuteKeyQuery(prefix string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	// Most important log: new request, prefix, and relevant parts of req
	logger.Debug("[KeyIterator] Query",
		"prefix", prefix,
		"before", req.Before,
		"after", req.After,
		"anchor", req.Anchor,
		"limit", req.Limit,
		"sort_by", req.SortBy,
	)

	var iter *pebble.Iterator
	var err error

	if prefix == "" {
		// if no prefix, scan entire database
		iter, err = ki.db.NewIter(&pebble.IterOptions{})
	} else {
		lowerBound := []byte(prefix)
		upperBound := nextPrefix([]byte(prefix))
		iter, err = ki.db.NewIter(&pebble.IterOptions{
			LowerBound: lowerBound,
			UpperBound: upperBound,
		})
	}
	if err != nil {
		logger.Error("[KeyIterator] Failed to create iterator", "error", err)
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	var keys []string
	var response pagination.PaginationResponse

	switch {
	case req.Anchor != "":
		beforeKeys, hasMoreBefore, err := ki.fetchBefore(iter, req.Anchor, req.Limit/2)
		if err != nil {
			logger.Error("[KeyIterator] fetchBefore error (anchor)", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, hasMoreAfter, err := ki.fetchAfter(iter, req.Anchor, req.Limit-len(beforeKeys))
		if err != nil {
			logger.Error("[KeyIterator] fetchAfter error (anchor)", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		keys = append(beforeKeys, req.Anchor)
		keys = append(keys, afterKeys...)
		response = pagination.PaginationResponse{
			HasBefore: hasMoreBefore,
			HasAfter:  hasMoreAfter,
			Count:     len(keys),
			Total:     ki.getTotalCount(iter),
		}
	case req.Before != "" && req.After != "":
		beforeKeys, hasMoreBefore, err := ki.fetchBefore(iter, req.Before, req.Limit/2)
		if err != nil {
			logger.Error("[KeyIterator] fetchBefore error (before+after)", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, hasMoreAfter, err := ki.fetchAfter(iter, req.After, req.Limit-len(beforeKeys))
		if err != nil {
			logger.Error("[KeyIterator] fetchAfter error (before+after)", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		keys = append(beforeKeys, afterKeys...)
		response = pagination.PaginationResponse{
			HasBefore: hasMoreBefore,
			HasAfter:  hasMoreAfter,
			Count:     len(keys),
			Total:     ki.getTotalCount(iter),
		}
	case req.Before != "":
		keys, response.HasBefore, err = ki.fetchBefore(iter, req.Before, req.Limit)
		if err != nil {
			logger.Error("[KeyIterator] fetchBefore error", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasAfter, _ = ki.checkHasAfter(iter, req.Before)
		response.Count = len(keys)
		response.Total = ki.getTotalCount(iter)
	case req.After != "":
		keys, response.HasAfter, err = ki.fetchAfter(iter, req.After, req.Limit)
		if err != nil {
			logger.Error("[KeyIterator] fetchAfter error", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasBefore, _ = ki.checkHasBefore(iter, req.After)
		response.Count = len(keys)
		response.Total = ki.getTotalCount(iter)
	default:
		// Only keep this log for initial load
		// (Can be useful for debugging first pagination experience)
		logger.Debug("[KeyIterator] Initial load", "limit", req.Limit)
		keys, response, err = ki.fetchInitialLoad(iter, req)
		if err != nil {
			logger.Error("[KeyIterator] fetchInitialLoad error", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
	}

	// Use keys as-is from iterator (no sorting needed for admin keys)
	sortedKeys := keys

	// Set navigation anchors based on query type (preserve HasBefore/HasAfter from fetch functions)
	if len(keys) > 0 {
		switch {
		case req.Before != "":
			// Before query: keys are newest→oldest
			response.BeforeAnchor = keys[len(keys)-1] // Last item (oldest) to get previous page
			response.AfterAnchor = keys[0]            // First item (newest) to get next page
		case req.After != "":
			// After query: keys are oldest→newest
			response.BeforeAnchor = keys[0]          // First item (oldest) to get previous page
			response.AfterAnchor = keys[len(keys)-1] // Last item (newest) to get next page
		default:
			// Initial load: keys are newest→oldest
			response.BeforeAnchor = keys[0]          // First item (newest) - for going to newer items
			response.AfterAnchor = keys[len(keys)-1] // Last item (oldest) - for going to older items
			// HasBefore/HasAfter already correctly set by fetchInitialLoad
		}
	}
	// Only print anchors if there are results (this is helpful for anchor debugging)
	if len(sortedKeys) > 0 {
		logger.Debug("[KeyIterator] Result anchors",
			"before_anchor", response.BeforeAnchor,
			"after_anchor", response.AfterAnchor,
			"count", response.Count,
			"has_before", response.HasBefore,
			"has_after", response.HasAfter)
	} else {
		logger.Debug("[KeyIterator] No keys returned from query")
	}

	return sortedKeys, response, nil
}

func (ki *KeyIterator) fetchInitialLoad(iter *pebble.Iterator, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	var reverseItems []string

	ok := iter.Last() // Start from newest key (lexicographically greatest)

	if ok {
		logger.Debug("[KeyIterator] fetchInitialLoad start", "first_key", string(iter.Key()), "limit", req.Limit)
	} else {
		logger.Debug("[KeyIterator] fetchInitialLoad", "no_keys_found", true)
	}

	// Collect keys from newest → oldest
	for ok && len(reverseItems) < req.Limit {
		key := string(iter.Key())
		reverseItems = append(reverseItems, key)
		ok = iter.Prev() // move backward (older)
	}

	// If Prev() succeeded and we still had valid key after reaching limit → more "before" keys exist
	hasBefore := ok && len(reverseItems) >= req.Limit

	// Reverse slice so output is [oldest...newest]
	items := make([]string, len(reverseItems))
	for i := range reverseItems {
		items[i] = reverseItems[len(reverseItems)-1-i]
	}

	if len(items) > 0 {
		logger.Debug("[KeyIterator] fetchInitialLoad complete",
			"oldest_key", items[0],
			"newest_key", items[len(items)-1],
			"count", len(items))
	}

	response := pagination.PaginationResponse{
		HasBefore: hasBefore, // There are older (before) items
		HasAfter:  false,     // Nothing newer; we're at the newest end
		Count:     len(items),
		Total:     ki.getTotalCount(iter),
	}

	return items, response, nil
}

func (ki *KeyIterator) fetchBefore(iter *pebble.Iterator, reference string, limit int) ([]string, bool, error) {
	var items []string
	referenceKey := []byte(reference)

	keyIter := iter.SeekLT(referenceKey)

	for keyIter && len(items) < limit {
		key := string(iter.Key())
		items = append(items, key)
		keyIter = iter.Prev()
	}

	hasMore := keyIter

	return items, hasMore, iter.Error()
}

func (ki *KeyIterator) fetchAfter(iter *pebble.Iterator, reference string, limit int) ([]string, bool, error) {
	var items []string

	keyIter := iter.SeekGE([]byte(reference))
	if keyIter && string(iter.Key()) == reference {
		keyIter = iter.Next()
	}

	for keyIter && len(items) < limit {
		key := string(iter.Key())
		items = append(items, key)
		keyIter = iter.Next()
	}

	hasMore := keyIter

	return items, hasMore, iter.Error()
}

func (ki *KeyIterator) checkHasBefore(iter *pebble.Iterator, reference string) (bool, error) {
	referenceKey := []byte(reference)
	keyIter := iter.SeekLT(referenceKey)
	return keyIter, iter.Error()
}

func (ki *KeyIterator) checkHasAfter(iter *pebble.Iterator, reference string) (bool, error) {
	keyIter := iter.SeekGE([]byte(reference))
	if keyIter && string(iter.Key()) == reference {
		keyIter = iter.Next()
	}
	return keyIter, iter.Error()
}

func (ki *KeyIterator) getTotalCount(iter *pebble.Iterator) int {
	count := 0
	keyIter := iter.First()
	for keyIter {
		count++
		keyIter = iter.Next()
	}
	return count
}

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
