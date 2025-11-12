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

	switch {
	case req.Anchor != "":
		beforeKeys, _, err := ki.fetchBefore(iter, req.Anchor, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchBefore error (anchor)", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, _, err := ki.fetchAfter(iter, req.Anchor, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchAfter error (anchor)", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		keys = append(beforeKeys, req.Anchor)
		keys = append(keys, afterKeys...)

		// Sort entire combined array to oldest→newest for chat display (newest at bottom)
		sorter := NewKeySorter()
		keys = sorter.SortKeys(keys, req.SortBy, &pagination.PaginationResponse{})

	case req.Before != "" && req.After != "":
		beforeKeys, _, err := ki.fetchBefore(iter, req.Before, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchBefore error (before+after)", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		afterKeys, _, err := ki.fetchAfter(iter, req.After, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchAfter error (before+after)", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		keys = append(beforeKeys, afterKeys...)

		// Sort entire combined array to oldest→newest for chat display (newest at bottom)
		sorter := NewKeySorter()
		keys = sorter.SortKeys(keys, req.SortBy, &pagination.PaginationResponse{})

	case req.Before != "":
		keys, _, err = ki.fetchBefore(iter, req.Before, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchBefore error", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		// Don't sort - fetchBefore returns keys in correct order already

	case req.After != "":
		keys, _, err = ki.fetchAfter(iter, req.After, req.Limit)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchAfter error", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
		// Don't sort - fetchAfter returns keys in correct order already

	default:
		// Only keep this log for initial load
		// (Can be useful for debugging first pagination experience)
		logger.Debug("[FrontendKeyIterator] Initial load", "limit", req.Limit)
		keys, _, err = ki.fetchInitialLoad(iter, req)
		if err != nil {
			logger.Error("[FrontendKeyIterator] fetchInitialLoad error", "error", err)
			return nil, pagination.PaginationResponse{}, err
		}
	}

	// Return empty pagination response - business iterators will handle pagination
	response := pagination.PaginationResponse{
		HasBefore: false,
		HasAfter:  false,
		Count:     len(keys),
		Total:     0, // Business iterators will calculate total
	}

	logger.Debug("[FrontendKeyIterator] Raw keys returned",
		"count", len(keys),
		"prefix", prefix)

	return keys, response, nil
}

func (ki *KeyIterator) fetchInitialLoad(iter *pebble.Iterator, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	var keys []string

	valid := iter.Last()

	for valid && len(keys) < req.Limit {
		key := string(iter.Key())
		keys = append(keys, key)
		valid = iter.Prev()
	}

	// Return empty pagination response - business iterators will handle pagination
	response := pagination.PaginationResponse{
		HasBefore: false,
		HasAfter:  false,
		Count:     len(keys),
		Total:     0, // Business iterators will calculate total
	}

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
