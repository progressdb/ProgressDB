package mi

import (
	"encoding/json"

	"progressdb/pkg/models"
	"progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/encryption"
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

	if len(messageKeys) == 0 {
		return messages, nil
	}

	// Get KMS metadata once for the thread (all messages belong to same thread)
	// Parse first message to get thread key
	firstValue, closer, err := storedb.Client.Get([]byte(messageKeys[0]))
	if err != nil {
		return messages, err
	}
	defer closer.Close()

	var firstMessage models.Message
	if err := json.Unmarshal(firstValue, &firstMessage); err != nil {
		return messages, err
	}

	// Get KMS metadata once
	kmsMeta, err := encryption.GetThreadKMS(firstMessage.Thread)
	if err != nil {
		return messages, err
	}

	// Now fetch and decrypt all messages using the same KMS metadata
	for _, messageKey := range messageKeys {
		// Get encrypted message data directly from store
		value, closer, err := storedb.Client.Get([]byte(messageKey))
		if err != nil {
			// Add log for store fetch error
			// You may want to use your preferred logger, but for demonstration:
			// log.Printf("Failed to load message key %s: %v", messageKey, err)
			println("[mi/fetcher] Failed to load message key:", messageKey, "err:", err.Error())
			continue // Skip messages that can't be loaded
		}
		defer closer.Close()

		// Decrypt message data using the cached KMS metadata
		decryptedData, err := encryption.DecryptMessageData(kmsMeta, value)
		if err != nil {
			// log.Printf("Failed to decrypt message key %s: %v", messageKey, err)
			println("[mi/fetcher] Failed to decrypt message key:", messageKey, "err:", err.Error())
			continue // Skip messages that can't be decrypted
		}

		// Unmarshal decrypted data
		var message models.Message
		if err := json.Unmarshal(decryptedData, &message); err != nil {
			// log.Printf("Failed to unmarshal decrypted data for message key %s: %v", messageKey, err)
			println("[mi/fetcher] Failed to unmarshal decrypted data for message key:", messageKey, "err:", err.Error())
			continue // Skip invalid decrypted message data
		}

		messages = append(messages, message)
	}

	return messages, nil
}
