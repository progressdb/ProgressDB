package mi

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/iterator/admin/ki"
	"progressdb/pkg/store/pagination"
)

// MessageIterator handles message-specific pagination and counting
type MessageIterator struct {
	db      *pebble.DB
	keyIter *ki.KeyIterator
}

// NewMessageIterator creates a new message iterator
func NewMessageIterator(db *pebble.DB) *MessageIterator {
	return &MessageIterator{
		db:      db,
		keyIter: ki.NewKeyIterator(db),
	}
}

// GetMessageCount gets the total message count for a thread using index system
func (mi *MessageIterator) GetMessageCount(threadKey string) (int, error) {
	// Use the existing index system which properly tracks message counts
	indexes, err := indexdb.GetThreadMessageIndexData(threadKey)
	if err != nil {
		// If index doesn't exist, return 0 (admin doesn't need manual counting)
		return 0, nil
	}

	// The End field in ThreadMessageIndexes is the total message count
	return int(indexes.End), nil
}

// ExecuteMessageQuery executes message pagination for a specific thread
func (mi *MessageIterator) ExecuteMessageQuery(threadKey string, req pagination.PaginationRequest) ([]string, pagination.PaginationResponse, error) {
	// Generate message key prefix for this thread
	messagePrefix := fmt.Sprintf("t:%s:m:", threadKey)

	// Use the key iterator for pure key-based pagination
	keys, response, err := mi.keyIter.ExecuteKeyQuery(messagePrefix, req)
	if err != nil {
		return nil, pagination.PaginationResponse{}, fmt.Errorf("failed to execute key query: %w", err)
	}

	// Get total count of all messages for this thread
	total, err := mi.GetMessageCount(threadKey)
	if err != nil {
		// Log error but don't fail request
		total = 0
	}
	response.Total = total

	return keys, response, nil
}
