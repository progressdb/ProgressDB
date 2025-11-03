package mi

import (
	"encoding/json"

	"progressdb/pkg/models"
	"progressdb/pkg/store/db/storedb"
)

// MessageFetcher handles fetching message data from keys
type MessageFetcher struct{}

// NewMessageFetcher creates a new message fetcher
func NewMessageFetcher() *MessageFetcher {
	return &MessageFetcher{}
}

// FetchMessages fetches message data for the given keys
func (mf *MessageFetcher) FetchMessages(messageKeys []string) ([]models.Message, error) {
	messages := make([]models.Message, 0, len(messageKeys))

	for _, messageKey := range messageKeys {
		// Get key value directly from store
		value, closer, err := storedb.Client.Get([]byte(messageKey))
		if err != nil {
			continue // Skip messages that can't be loaded
		}
		defer closer.Close()

		var message models.Message
		if err := json.Unmarshal(value, &message); err != nil {
			continue // Skip invalid message data
		}

		messages = append(messages, message)
	}

	return messages, nil
}
