package mi

import (
	"encoding/json"

	"progressdb/pkg/models"
	"progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/encryption"
)

type MessageFetcher struct{}

func NewMessageFetcher() *MessageFetcher {
	return &MessageFetcher{}
}

func (mf *MessageFetcher) FetchMessages(messageKeys []string) ([]models.Message, error) {
	messages := make([]models.Message, 0, len(messageKeys))

	if len(messageKeys) == 0 {
		return messages, nil
	}

	firstValue, closer, err := storedb.Client.Get([]byte(messageKeys[0]))
	if err != nil {
		return messages, err
	}
	defer closer.Close()

	var firstMessage models.Message
	if err := json.Unmarshal(firstValue, &firstMessage); err != nil {
		return messages, err
	}

	kmsMeta, err := encryption.GetThreadKMS(firstMessage.Thread)
	if err != nil {
		return messages, err
	}

	for _, messageKey := range messageKeys {
		value, closer, err := storedb.Client.Get([]byte(messageKey))
		if err != nil {
			println("[mi/fetcher] Failed to load message key:", messageKey, "err:", err.Error())
			continue
		}
		defer closer.Close()

		decryptedData, err := encryption.DecryptMessageData(kmsMeta, value)
		if err != nil {
			println("[mi/fetcher] Failed to decrypt message key:", messageKey, "err:", err.Error())
			continue
		}

		var message models.Message
		if err := json.Unmarshal(decryptedData, &message); err != nil {
			println("[mi/fetcher] Failed to unmarshal decrypted data for message key:", messageKey, "err:", err.Error())
			continue
		}

		messages = append(messages, message)
	}

	return messages, nil
}
