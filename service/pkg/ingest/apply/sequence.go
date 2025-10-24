package apply

import (
	"bytes"

	"progressdb/pkg/logger"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
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
	// if it's already a final key (has sequence), return as-is
	if !keys.IsProvisionalMessageKey(messageKey) {
		return messageKey, nil
	}

	// for provisional:

	// check in-memory mapping first
	if finalKey, ok := m.provisionalToFinalKeys[messageKey]; ok {
		return finalKey, nil
	}

	// fallback to store db itself
	if storedb.Client == nil {
		logger.Debug("store_not_ready", "provisional", messageKey)
		return messageKey, nil
	}

	// create prefix for provisional key + ":" to find the sequenced key
	// it is going to be just one key because of the timestamp
	prefix := messageKey + ":"

	iter, err := storedb.Client.NewIter(&pebble.IterOptions{})
	if err != nil {
		logger.Error("iterator_create_failed", "error", err)
		return messageKey, nil
	}
	defer iter.Close()

	// seek to the prefix
	iter.SeekGE([]byte(prefix))

	if iter.Valid() && bytes.HasPrefix(iter.Key(), []byte(prefix)) {
		// found the actual sequenced key
		finalKey := string(iter.Key())
		logger.Debug("found prov <> final message key", "provisional", messageKey, "final", finalKey)

		// cache it in memory for future lookups
		m.provisionalToFinalKeys[messageKey] = finalKey
		return finalKey, nil
	}

	// no sequenced key found in database
	logger.Error("no prov <> final message key found", "provisional", messageKey)
	return messageKey, nil
}

func (m *MessageSequencer) IsProvisionalMessageKey(messageKey string) bool {
	return keys.IsProvisionalMessageKey(messageKey)
}

func (m *MessageSequencer) Reset() {
	m.provisionalToFinalKeys = make(map[string]string)
}
