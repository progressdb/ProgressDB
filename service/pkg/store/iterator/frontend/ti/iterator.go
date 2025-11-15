package ti

import (
	"fmt"
	"strings"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
)

type ThreadIterator struct {
	db      *pebble.DB
	keys    *KeyManager
	fetcher *ThreadFetcher
	sorter  *ThreadSorter
	paging  *PageManager
}

func NewThreadIterator(db *pebble.DB) *ThreadIterator {
	keys := NewKeyManager(db)

	return &ThreadIterator{
		db:      db,
		keys:    keys,
		fetcher: NewThreadFetcher(),
		sorter:  NewThreadSorter(),
		paging:  NewPageManager(keys),
	}
}

func (ti *ThreadIterator) ExecuteThreadQuery(userID string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	// 1. Generate user thread prefix
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to generate user thread prefix: %w", err)
	}

	// 2. Transform thread keys to relationship keys for the keys layer
	transformedReq := ti.transformRequestKeys(userID, req)

	// 3. Get valid relationship keys (deletion-aware)
	relationshipKeys, err := ti.keys.ExecuteKeyQuery(userID, userThreadPrefix, transformedReq)
	if err != nil {
		logger.Error("Key query failed", "userID", userID, "error", err)
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	// 5. Convert to thread keys for fetching
	threadKeys := make([]string, 0, len(relationshipKeys))
	for _, relKey := range relationshipKeys {
		if parsed, err := keys.ParseUserOwnsThread(relKey); err == nil {
			threadKeys = append(threadKeys, parsed.ThreadKey)
		}
	}

	// 6. Fetch thread data
	threads, err := ti.fetcher.FetchThreads(threadKeys, userID)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to fetch threads: %w", err)
	}

	// 7. Sort threads
	threads = ti.sorter.SortThreads(threads, req.SortBy)

	// 8. Calculate pagination metadata
	total, err := ti.getTotalThreadCount(userID)
	if err != nil {
		total = 0
	}

	paginationResp := ti.paging.CalculatePagination(threads, total, req, userID)

	// 9. Return thread keys for API response
	finalThreadKeys := make([]string, len(threads))
	for i, thread := range threads {
		finalThreadKeys[i] = thread.Key
	}

	logger.Debug("ThreadIterator completed",
		"requested", req.Limit,
		"returned", len(finalThreadKeys),
		"total", total)

	return finalThreadKeys, paginationResp, nil
}

func (ti *ThreadIterator) transformRequestKeys(userID string, req pagination.PaginationRequest) pagination.PaginationRequest {
	transformed := req

	if req.Before != "" {
		if !strings.HasPrefix(req.Before, "rel:u:") {
			// Convert thread key to relationship key
			transformed.Before = ti.threadKeyToRelKey(userID, req.Before)
		}
	}
	if req.After != "" {
		if !strings.HasPrefix(req.After, "rel:u:") {
			transformed.After = ti.threadKeyToRelKey(userID, req.After)
		}
	}
	if req.Anchor != "" {
		if !strings.HasPrefix(req.Anchor, "rel:u:") {
			transformed.Anchor = ti.threadKeyToRelKey(userID, req.Anchor)
		}
	}

	return transformed
}

func (ti *ThreadIterator) threadKeyToRelKey(userID, threadKey string) string {
	if strings.HasPrefix(threadKey, "t:") {
		threadTS := strings.TrimPrefix(threadKey, "t:")
		return fmt.Sprintf(keys.RelUserOwnsThread, userID, threadTS)
	}
	return threadKey
}

func (ti *ThreadIterator) getTotalThreadCount(userID string) (int, error) {
	// Get total relationship keys count (includes deleted threads)
	totalRelKeys, err := ti.getTotalRelationshipKeysCount(userID)
	if err != nil {
		return 0, err
	}

	// Count deleted threads for this user
	deletedCount, err := ti.getDeletedThreadCount(userID)
	if err != nil {
		return 0, err
	}

	activeCount := totalRelKeys - deletedCount
	if activeCount < 0 {
		activeCount = 0 // Safety check
	}

	return activeCount, nil
}

// getTotalRelationshipKeysCount counts all user-thread relationship keys
func (ti *ThreadIterator) getTotalRelationshipKeysCount(userID string) (int, error) {
	userThreadPrefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return 0, err
	}

	const batchSize = 1000
	totalCount := 0
	offset := ""

	for {
		req := pagination.PaginationRequest{
			Limit: batchSize,
		}
		if offset != "" {
			req.After = offset
		}

		relationshipKeys, err := ti.keys.ExecuteKeyQuery(userID, userThreadPrefix, req)
		if err != nil {
			return totalCount, err
		}

		totalCount += len(relationshipKeys)

		if len(relationshipKeys) < batchSize {
			break
		}

		offset = relationshipKeys[len(relationshipKeys)-1]
	}

	return totalCount, nil
}

func (ti *ThreadIterator) getDeletedThreadCount(userID string) (int, error) {
	// Count delete markers with prefix "del:t:" for threads owned by this user
	// We need to scan all thread delete markers and check if they belong to this user
	deletePrefix := keys.GenSoftDeletePrefix() + keys.GenThreadMetadataPrefix()

	// Use StoreDB iterator to scan thread delete markers directly
	iter, err := storedb.Client.NewIter(&pebble.IterOptions{
		LowerBound: []byte(deletePrefix),
		UpperBound: nextPrefix([]byte(deletePrefix)),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create iterator for thread delete markers: %w", err)
	}
	defer iter.Close()

	count := 0
	for iter.Valid() {
		deleteMarkerKey := string(iter.Key())

		// Extract thread key from delete marker (del:t:{thread_key} -> t:{thread_key})
		threadKey := deleteMarkerKey[4:] // Remove "del:" prefix

		// Check if this deleted thread belongs to user by checking relationship
		userThreadRelKey, err := keys.GenUserThreadRelPrefix(userID)
		if err == nil {
			userThreadRelKey += threadKey
			_, err = indexdb.GetKey(userThreadRelKey)
			if err == nil {
				// Relationship exists = user owns this thread
				count++
			}
		}

		iter.Next()
	}

	return count, nil
}
