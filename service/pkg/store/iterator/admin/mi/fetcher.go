package mi

import (
	"encoding/json"

	"progressdb/pkg/models"
	message_store "progressdb/pkg/store/features/messages"
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
		messageData, err := message_store.GetLatestMessageData(messageKey)
		if err != nil {
			continue // Skip messages that can't be loaded
		}

		var message models.Message
		if err := json.Unmarshal([]byte(messageData), &message); err != nil {
			continue // Skip invalid message data
		}

		messages = append(messages, message)
	}

	return messages, nil
}
