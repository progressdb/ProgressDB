package ti

import (
	"fmt"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/iterator/admin/ki"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
)

type ThreadIterator struct {
	db      *pebble.DB
	keyIter *ki.KeyIterator
}

func NewThreadIterator(db *pebble.DB) *ThreadIterator {
	return &ThreadIterator{
		db:      db,
		keyIter: ki.NewKeyIterator(db),
	}
}

func (ti *ThreadIterator) ExecuteThreadQuery(userID string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	logger.Debug("Admin ThreadIterator query started",
		"userID", userID,
		"limit", req.Limit,
		"after", req.After,
		"before", req.Before,
		"sortBy", req.SortBy)

	// Generate user thread relationship prefix
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		logger.Error("Admin ThreadIterator failed to generate prefix", "userID", userID, "error", err)
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to generate user thread prefix: %w", err)
	}

	// Use key iterator for pure key-based pagination on relationship keys
	relationshipKeys, response, err := ti.keyIter.ExecuteKeyQuery(userThreadPrefix, req)
	if err != nil {
		logger.Error("Admin ThreadIterator key query failed", "userID", userID, "error", err)
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	logger.Debug("Admin ThreadIterator query executed",
		"userID", userID,
		"relationshipKeysFound", len(relationshipKeys),
		"hasAfter", response.HasAfter,
		"hasBefore", response.HasBefore)

	// Get total count of all threads for this user
	total, err := ti.getTotalThreadCount(userID)
	if err != nil {
		// Log error but don't fail request
		total = 0
	}
	response.Total = total

	// Convert relationship keys to thread keys (no sorting needed)
	threadKeys := make([]string, 0, len(relationshipKeys))

	for _, relKey := range relationshipKeys {
		parsed, err := keys.ParseUserOwnsThread(relKey)
		if err != nil {
			continue
		}

		threadKeys = append(threadKeys, parsed.ThreadKey)
	}

	// Return keys as-is from KeyIterator (already in lexicographical order)
	// Anchors will be set by main KeyIterator logic
	return threadKeys, response, nil
}

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
