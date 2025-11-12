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
	logger.Debug("[FrontendKeyIterator] Query",
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
		logger.Error("[FrontendKeyIterator] Failed to create iterator", "error", err)
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	var keys []string
	var response pagination.PaginationResponse

	switch {
	case req.Anchor != "":
		beforeKeys, hasMoreBefore, err := ki.fetchBefore(iter, req.Anchor, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchBefore error (anchor)", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, hasMoreAfter, err := ki.fetchAfter(iter, req.Anchor, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchAfter error (anchor)", "error", err)
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

		// Sort entire combined array to oldest→newest for chat display (newest at bottom)
		sorter := NewKeySorter()
		keys = sorter.SortKeys(keys, req.SortBy, &response)

	case req.Before != "" && req.After != "":
		beforeKeys, hasMoreBefore, err := ki.fetchBefore(iter, req.Before, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchBefore error (before+after)", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, hasMoreAfter, err := ki.fetchAfter(iter, req.After, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchAfter error (before+after)", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		keys = append(beforeKeys, afterKeys...)
		response = pagination.PaginationResponse{
			HasBefore: hasMoreBefore,
			HasAfter:  hasMoreAfter,
			Count:     len(keys),
			Total:     ki.getTotalCount(iter),
		}

		// Sort entire combined array to oldest→newest for chat display (newest at bottom)
		sorter := NewKeySorter()
		keys = sorter.SortKeys(keys, req.SortBy, &response)

	case req.Before != "":
		keys, response.HasBefore, err = ki.fetchBefore(iter, req.Before, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchBefore error", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasAfter, _ = ki.checkHasAfter(iter, req.Before)
		response.Count = len(keys)
		response.Total = ki.getTotalCount(iter)

		// Don't sort - fetchBefore returns keys in correct order already

	case req.After != "":
		keys, response.HasAfter, err = ki.fetchAfter(iter, req.After, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchAfter error", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		response.HasBefore, _ = ki.checkHasBefore(iter, req.After)
		response.Count = len(keys)
		response.Total = ki.getTotalCount(iter)

		// Don't sort - fetchAfter returns keys in correct order already

	default:
		// Only keep this log for initial load
		// (Can be useful for debugging first pagination experience)
		logger.Debug("[FrontendKeyIterator] Initial load", "limit", req.Limit)
		keys, response, err = ki.fetchInitialLoad(iter, req)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchInitialLoad error", "error", err)
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
		logger.Debug("[FrontendKeyIterator] Result anchors",
			"before_anchor", response.BeforeAnchor,
			"after_anchor", response.AfterAnchor,
			"count", response.Count,
			"has_before", response.HasBefore,
			"has_after", response.HasAfter)
	} else {
		logger.Debug("[FrontendKeyIterator] No keys returned from query")
	}

	return sortedKeys, response, nil
}

func (ki *KeyIterator) fetchInitialLoad(iter *pebble.Iterator, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	var keys []string

	valid := iter.Last()

	for valid && len(keys) < req.Limit {
		key := string(iter.Key())
		keys = append(keys, key)
		valid = iter.Prev()
	}

	response := pagination.PaginationResponse{
		HasBefore: valid,
		HasAfter:  false,
		Count:     len(keys),
		Total:     ki.getTotalCount(iter),
	}

	// Anchors will be set by main logic after sorting

	return keys, response, nil
}

func (ki *KeyIterator) fetchBefore(iter *pebble.Iterator, reference string, limit int) ([]string, bool, error) {
	var keys []string
	referenceKey := []byte(reference)

	valid := iter.SeekLT(referenceKey)

	for valid && len(keys) < limit {
		key := string(iter.Key())
		keys = append(keys, key)
		logger.Debug("[fetchBefore] Key found", "key", key)
		valid = iter.Prev()
	}

	hasMore := valid && len(keys) >= limit

	return keys, hasMore, iter.Error()
}

func (ki *KeyIterator) fetchAfter(iter *pebble.Iterator, reference string, limit int) ([]string, bool, error) {
	var keys []string

	valid := iter.SeekGE([]byte(reference))
	if valid && string(iter.Key()) == reference {
		valid = iter.Next()
	}

	for valid && len(keys) < limit {
		key := string(iter.Key())
		keys = append(keys, key)
		valid = iter.Next()
	}

	hasMore := valid && len(keys) >= limit

	return keys, hasMore, iter.Error()
}

func (ki *KeyIterator) checkHasBefore(iter *pebble.Iterator, reference string) (bool, error) {
	referenceKey := []byte(reference)
	valid := iter.SeekLT(referenceKey)
	return valid, nil
}

func (ki *KeyIterator) checkHasAfter(iter *pebble.Iterator, reference string) (bool, error) {
	valid := iter.SeekGE([]byte(reference))
	if valid && string(iter.Key()) == reference {
		valid = iter.Next()
	}
	return valid, nil
}

func (ki *KeyIterator) getTotalCount(iter *pebble.Iterator) int {
	count := 0
	valid := iter.First()

	for valid {
		count++
		valid = iter.Next()
	}

	return count
}

// nextPrefix returns the next prefix for range scanning
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
