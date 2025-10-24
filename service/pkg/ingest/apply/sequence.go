package apply

import (
	"progressdb/pkg/logger"
	"progressdb/pkg/store/keys"
)

type MessageSequencer struct {
	provisionalToFinalKeys map[string]string
}

func NewMessageSequencer() *MessageSequencer {
	return &MessageSequencer{
		provisionalToFinalKeys: make(map[string]string),
	}
}

func (m *MessageSequencer) MapProvisionalToFinalMessageKey(provisionalKey, finalKey string) {
	m.provisionalToFinalKeys[provisionalKey] = finalKey
	logger.Debug("mapped_provisional_message", "provisional", provisionalKey, "final", finalKey)
}

func (m *MessageSequencer) GetFinalMessageKey(messageKey string) (string, error) {
	if keys.IsProvisionalMessageKey(messageKey) {
		return messageKey, nil
	}
	if finalKey, ok := m.provisionalToFinalKeys[messageKey]; ok {
		return finalKey, nil
	}
	logger.Debug("message_key_not_resolved", "provisional", messageKey)
	return messageKey, nil
}

func (m *MessageSequencer) IsProvisionalMessageKey(messageKey string) bool {
	return keys.IsProvisionalMessageKey(messageKey)
}

func (m *MessageSequencer) Reset() {
	m.provisionalToFinalKeys = make(map[string]string)
}
