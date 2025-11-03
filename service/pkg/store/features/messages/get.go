package messages

import (
	"fmt"

	"progressdb/pkg/state/telemetry"
	"progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"
)

func GetMessageData(messageKey string) (string, error) {
	tr := telemetry.Track("storedb.get_message")
	defer tr.Finish()

	tr.Mark("validate_message_key")
	if !keys.IsMessageKey(messageKey) {
		return "", fmt.Errorf("invalid message key: %s", messageKey)
	}

	tr.Mark("get_message")
	data, err := storedb.GetKey(messageKey)
	if err != nil {
		return "", fmt.Errorf("failed to get message data: %w", err)
	}

	return data, nil
}
