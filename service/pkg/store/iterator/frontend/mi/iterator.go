package mi

import (
	"fmt"
	"strconv"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

type MessageIterator struct {
	db      *pebble.DB
	keys    *KeyManager
	fetcher *MessageFetcher
	sorter  *MessageSorter
	paging  *PageManager
}

func NewMessageIterator(db *pebble.DB) *MessageIterator {
	keys := NewKeyManager()

	return &MessageIterator{
		db:      db,
		keys:    keys,
		fetcher: NewMessageFetcher(),
		sorter:  NewMessageSorter(),
		paging:  NewPageManager(keys),
	}
}

func (mi *MessageIterator) ExecuteMessageQuery(threadKey string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	if !keys.IsThreadKey(threadKey) {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("invalid thread key: %s", threadKey)
	}

	// 1. Generate message prefix
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// 2. Get valid message keys (deletion-aware - handled by KeyManager)
	messageKeys, err := mi.keys.ExecuteKeyQuery(threadKey, messagePrefix, req)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	println("[mi/iterator] ExecuteKeyQuery returned", len(messageKeys), "keys")
	for i, key := range messageKeys {
		println("[mi/iterator]  key[", i, "]:", key)
	}

	// 3. Fetch message data (keys are already filtered for deletions)
	messages, err := mi.fetcher.FetchMessages(messageKeys)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to fetch messages: %w", err)
	}

	println("[mi/iterator] FetchMessages returned", len(messages), "messages")
	for i, msg := range messages {
		println("[mi/iterator]  message[", i, "]:", msg.Key)
	}

	// 4. No sorting needed - keys.go handles proper ordering for all query types

	// 5. Calculate pagination metadata
	total, err := mi.GetMessageCountExcludingDeleted(threadKey)
	if err != nil {
		total = 0
	}

	paginationResp := mi.paging.CalculatePagination(messages, total, req, threadKey)

	// 6. Return message keys for API response
	finalMessageKeys := make([]string, len(messages))
	for i, message := range messages {
		finalMessageKeys[i] = message.Key
	}

	println("[mi/iterator] finalMessageKeys has", len(finalMessageKeys), "keys")

	return finalMessageKeys, paginationResp, nil
}

func (mi *MessageIterator) GetMessageCountExcludingDeleted(threadKey string) (int, error) {
	// Get total message count from index keys
	totalCount, err := mi.getTotalMessageCountFromIndex(threadKey)
	if err != nil {
		// Fallback to old method if index keys don't exist
		return mi.countMessagesByIteration(threadKey)
	}

	// Count deleted messages only
	deletedCount, err := mi.getDeletedMessagesCount(threadKey)
	if err != nil {
		return 0, err
	}

	activeCount := totalCount - deletedCount
	if activeCount < 0 {
		activeCount = 0 // Safety check
	}

	return activeCount, nil
}

func (mi *MessageIterator) getTotalMessageCountFromIndex(threadKey string) (int, error) {
	// Get start sequence
	startKey := fmt.Sprintf(keys.ThreadMessageStart, threadKey)
	startSeqBytes, err := indexdb.GetKey(startKey)
	if err != nil {
		return 0, fmt.Errorf("failed to get message start index: %w", err)
	}

	// Get end sequence
	endKey := fmt.Sprintf(keys.ThreadMessageEnd, threadKey)
	endSeqBytes, err := indexdb.GetKey(endKey)
	if err != nil {
		return 0, fmt.Errorf("failed to get message end index: %w", err)
	}

	// Parse sequence numbers
	startSeq, err := strconv.ParseUint(string(startSeqBytes), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse start sequence: %w", err)
	}

	endSeq, err := strconv.ParseUint(string(endSeqBytes), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse end sequence: %w", err)
	}

	// Calculate total count: (end - start) + 1
	totalCount := int(endSeq - startSeq + 1)
	return totalCount, nil
}

func (mi *MessageIterator) getDeletedMessagesCount(threadKey string) (int, error) {
	// Count delete markers with prefix "del:{thread_key}:m:"
	threadMessagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return 0, fmt.Errorf("failed to generate thread message prefix: %w", err)
	}
	deletePrefix := keys.GenSoftDeletePrefix() + threadMessagePrefix

	// Use StoreDB iterator to scan delete markers directly
	iter, err := storedb.Client.NewIter(&pebble.IterOptions{
		LowerBound: []byte(deletePrefix),
		UpperBound: nextPrefix([]byte(deletePrefix)),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create iterator for delete markers: %w", err)
	}
	defer iter.Close()

	count := 0
	for iter.Valid() {
		iter.Next()
		count++
	}

	return count, nil
}

func (mi *MessageIterator) countMessagesByIteration(threadKey string) (int, error) {
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return 0, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	count := 0
	batchSize := 10000

	for {
		messageKeys, err := mi.keys.ExecuteKeyQuery(threadKey, messagePrefix, pagination.PaginationRequest{
			Limit: batchSize,
		})
		if err != nil {
			return 0, err
		}

		if len(messageKeys) == 0 {
			break
		}

		// Count non-deleted messages in this batch
		for _, messageKey := range messageKeys {
			deleteMarkerKey := keys.GenSoftDeleteMarkerKey(messageKey)
			_, err := indexdb.GetKey(deleteMarkerKey)
			if err != nil {
				if indexdb.IsNotFound(err) {
					// No soft delete marker found = not deleted
					count++
				}
				// Any other error = fail-safe, don't count
			}
		}

		if len(messageKeys) < batchSize {
			break
		}
	}

	return count, nil
}
