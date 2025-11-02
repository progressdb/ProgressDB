package ti

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/iterator/admin/ki"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

// ThreadIterator handles thread-specific pagination
type ThreadIterator struct {
	db      *pebble.DB
	keyIter *ki.KeyIterator
}

// NewThreadIterator creates a new thread iterator
func NewThreadIterator(db *pebble.DB) *ThreadIterator {
	return &ThreadIterator{
		db:      db,
		keyIter: ki.NewKeyIterator(db),
	}
}

// ExecuteThreadQuery executes a thread pagination query for a specific user
func (ti *ThreadIterator) ExecuteThreadQuery(userID string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	// Generate user thread relationship prefix
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to generate user thread prefix: %w", err)
	}

	// Use key iterator for pure key-based pagination on relationship keys
	relationshipKeys, response, err := ti.keyIter.ExecuteKeyQuery(userThreadPrefix, req)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	// Get total count of all threads for this user
	total, err := ti.getTotalThreadCount(userID)
	if err != nil {
		// Log error but don't fail request
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
