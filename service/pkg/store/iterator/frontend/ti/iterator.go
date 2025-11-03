package ti

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/iterator/frontend/ki"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

type ThreadIterator struct {
	db *pebble.DB
}

func NewThreadIterator(db *pebble.DB) *ThreadIterator {
	return &ThreadIterator{db: db}
}

func (ti *ThreadIterator) ExecuteThreadQuery(userID string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to generate user thread prefix: %w", err)
	}

	// Use the frontend key iterator for robust key iteration
	keyIter := ki.NewKeyIterator(ti.db)

	// Execute key query using the proven ki logic
	relationshipKeys, response, err := keyIter.ExecuteKeyQuery(userThreadPrefix, req)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	// Get total thread count
	total, err := ti.getTotalThreadCount(userID)
	if err != nil {
		total = 0
	}
	response.Total = total

	// Use thread sorter for final sorting (maintains frontend-specific sorting)
	sorter := NewThreadSorter()
	sortedKeys := sorter.SortKeys(relationshipKeys, req.SortBy, req.OrderBy, &response)

	// Convert relationship keys to thread keys
	threadKeys := make([]string, 0, len(sortedKeys))
	for _, relKey := range sortedKeys {
		parsed, err := keys.ParseUserOwnsThread(relKey)
		if err != nil {
			continue
		}
		threadKeys = append(threadKeys, parsed.ThreadKey)
	}

	// Update anchors to use thread keys instead of relationship keys
	if len(threadKeys) > 0 {
		response.StartAnchor = threadKeys[0]
		response.EndAnchor = threadKeys[len(threadKeys)-1]
	}

	return threadKeys, response, nil
}

func (ti *ThreadIterator) getTotalThreadCount(userID string) (int, error) {
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return 0, err
	}

	// Use frontend ki for consistent counting
	keyIter := ki.NewKeyIterator(ti.db)
	keys, _, err := keyIter.ExecuteKeyQuery(userThreadPrefix, pagination.PaginationRequest{Limit: 1000000})
	if err != nil {
		return 0, err
	}

	return len(keys), nil
}
