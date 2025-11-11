package mi

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/iterator/frontend/ki"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

type MessageIterator struct {
	db *pebble.DB
}

func NewMessageIterator(db *pebble.DB) *MessageIterator {
	return &MessageIterator{db: db}
}

func (mi *MessageIterator) GetMessageCount(threadKey string) (int, error) {
	indexes, err := indexdb.GetThreadMessageIndexData(threadKey)
	if err != nil {
		return mi.countMessagesManually(threadKey)
	}

	return int(indexes.End), nil
}

func (mi *MessageIterator) countMessagesManually(threadKey string) (int, error) {
	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return 0, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// Use frontend ki for consistent counting
	keyIter := ki.NewKeyIterator(mi.db)
	keys, _, err := keyIter.ExecuteKeyQuery(messagePrefix, pagination.PaginationRequest{Limit: 1000000})
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}

	return len(keys), nil
}

func (mi *MessageIterator) ExecuteMessageQuery(threadKey string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	if !keys.IsThreadKey(threadKey) {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("invalid thread key: %s", threadKey)
	}

	// Use the frontend key iterator for robust key iteration
	keyIter := ki.NewKeyIterator(mi.db)

	messagePrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to generate message prefix: %w", err)
	}

	// Execute key query using the proven ki logic
	messageKeys, response, err := keyIter.ExecuteKeyQuery(messagePrefix, req)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	// Get total message count
	total, err := mi.GetMessageCount(threadKey)
	if err != nil {
		total = 0
	}
	response.Total = total

	var sortedKeys []string

	// Only sort for initial load - before/after are already in correct order from ki
	if req.Before == "" && req.After == "" && req.Anchor == "" {
		sorter := NewMessageSorter()
		sortedKeys = sorter.SortKeys(messageKeys, req.SortBy, &response)
	} else {
		// Use keys as-is from ki (already in correct order)
		sortedKeys = messageKeys

	}

	return sortedKeys, response, nil
}
