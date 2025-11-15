package mi

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/db/indexdb"
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

// GetMessageCountExcludingDeleted returns the count of non-deleted messages in a thread
func (mi *MessageIterator) GetMessageCountExcludingDeleted(threadKey string) (int, error) {
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return 0, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// Use integrated logic for consistent counting with smaller batch size for efficiency
	count := 0
	batchSize := 10000

	for {
		// Process in batches to avoid loading all keys at once
		messageKeys, err := mi.keys.ExecuteKeyQuery(threadKey, messagePrefix, pagination.PaginationRequest{
			Limit: batchSize,
		})
		if err != nil {
			return 0, err
		}

		if len(messageKeys) == 0 {
			break // No more keys
		}

		// Count non-deleted messages in this batch
		for _, messageKey := range messageKeys {
			// Check if message is deleted
			deleteMarkerKey := keys.GenSoftDeleteMarkerKey(messageKey)
			_, err := indexdb.GetKey(deleteMarkerKey)
			if err != nil {
				if indexdb.IsNotFound(err) {
					// No soft delete marker found = not deleted
					count++
				}
				// Any other error = fail-safe, don't count
			}
			// Soft delete marker found = message is deleted, don't count
		}

		// If we got fewer keys than batch size, we're done
		if len(messageKeys) < batchSize {
			break
		}
	}

	return count, nil
}
