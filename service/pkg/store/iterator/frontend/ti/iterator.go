package ti

import (
	"fmt"
	"strings"

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

	// Transform thread keys to relationship keys for pagination
	if req.Before != "" {
		threadTS := strings.TrimPrefix(req.Before, "t:")
		req.Before = fmt.Sprintf(keys.RelUserOwnsThread, userID, threadTS)
	}
	if req.After != "" {
		threadTS := strings.TrimPrefix(req.After, "t:")
		req.After = fmt.Sprintf(keys.RelUserOwnsThread, userID, threadTS)
	}
	if req.Anchor != "" {
		threadTS := strings.TrimPrefix(req.Anchor, "t:")
		req.Anchor = fmt.Sprintf(keys.RelUserOwnsThread, userID, threadTS)
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

	var sortedKeys []string

	// Only sort for initial load - before/after are already in correct order from ki
	if req.Before == "" && req.After == "" && req.Anchor == "" {
		sorter := NewThreadSorter()
		sortedKeys = sorter.SortKeys(relationshipKeys, req.SortBy, &response)
	} else {
		// Use keys as-is from ki (already in correct order)
		sortedKeys = relationshipKeys

	}

	// Convert relationship keys to thread keys
	threadKeys := make([]string, 0, len(sortedKeys))
	for _, relKey := range sortedKeys {
		parsed, err := keys.ParseUserOwnsThread(relKey)
		if err != nil {
			continue
		}
		threadKeys = append(threadKeys, parsed.ThreadKey)
	}

	// Set navigation anchors based on final threadKeys order (oldest→newest for chat)
	if len(threadKeys) > 0 {
		response.BeforeAnchor = threadKeys[0]                // Oldest (first) for previous page
		response.AfterAnchor = threadKeys[len(threadKeys)-1] // Newest (last) for next page
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
