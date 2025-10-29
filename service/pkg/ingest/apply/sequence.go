package apply

import (
	"fmt"
	"strconv"
	"strings"

	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
)

type MessageSequencer struct {
	provisionalToFinalKeys map[string]string
	indexManager           *IndexManager
}

func NewMessageSequencer(im *IndexManager) *MessageSequencer {
	return &MessageSequencer{
		provisionalToFinalKeys: make(map[string]string),
		indexManager:           im,
	}
}

func (m *MessageSequencer) MapProvisionalToFinalMessageKey(provisionalKey, finalKey string) {
	m.provisionalToFinalKeys[provisionalKey] = finalKey
}

func (m *MessageSequencer) IsProvisionalMessageKey(messageKey string) bool {
	return keys.IsProvisionalMessageKey(messageKey)
}

func (m *MessageSequencer) Reset() {
	m.provisionalToFinalKeys = make(map[string]string)
}

func (m *MessageSequencer) GetFinalThreadKey(threadKey string) (string, error) {
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return "", fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", threadKey)
	}
	return threadKey, nil
}

func (m *MessageSequencer) ResolveMessageKey(msgKey string, finalKeyIfNew string) (string, error) {
	if msgKey == "" {
		return "", fmt.Errorf("msgKey cannot be empty")
	}
	if !keys.IsProvisionalMessageKey(msgKey) {
		return msgKey, nil
	}

	if finalKey, ok := m.provisionalToFinalKeys[msgKey]; ok {
		return finalKey, nil
	}

	if storedb.Client == nil {
		return m.generateNewSequencedKey(msgKey, finalKeyIfNew)
	}

	prefix := msgKey + ":"

	iter, err := storedb.Client.NewIter(nil)
	if err != nil {
		return m.generateNewSequencedKey(msgKey, finalKeyIfNew)
	}
	defer iter.Close()

	iter.SeekGE([]byte(prefix))

	if iter.Valid() && len(iter.Key()) > len(prefix) && string(iter.Key()[:len(prefix)]) == prefix {
		existingFinalKey := string(iter.Key())
		m.provisionalToFinalKeys[msgKey] = existingFinalKey
		return existingFinalKey, nil
	}

	return m.generateNewSequencedKey(msgKey, finalKeyIfNew)
}

func (m *MessageSequencer) generateNewSequencedKey(provisionalKey, finalKeyIfNew string) (string, error) {
	threadKey, err := m.extractThreadKeyFromKey(provisionalKey)
	if err != nil {
		m.MapProvisionalToFinalMessageKey(provisionalKey, finalKeyIfNew)
		return finalKeyIfNew, nil
	}

	sequence := m.indexManager.GetNextThreadSequence(threadKey)

	messageKey := m.extractMessageKeyFromKey(finalKeyIfNew)
	if messageKey == "" {
		messageKey = m.extractMessageKeyFromKey(provisionalKey)
	}

	finalKey := keys.GenMessageKey(threadKey, messageKey, sequence)
	m.MapProvisionalToFinalMessageKey(provisionalKey, finalKey)

	return finalKey, nil
}

func (m *MessageSequencer) extractThreadKeyFromKey(key string) (string, error) {
	if parts, err := keys.ParseThreadKey(key); err == nil {
		return parts.ThreadID, nil
	}

	if parts, err := keys.ParseMessageKey(key); err == nil {
		return parts.ThreadID, nil
	}

	return "", fmt.Errorf("unable to extract thread key from key: %s", key)
}

func (m *MessageSequencer) extractMessageKeyFromKey(key string) string {
	if parts, err := keys.ParseMessageKey(key); err == nil {
		return parts.MsgID
	}
	return ""
}

func extractSequenceFromKey(key string) uint64 {
	if parts, err := keys.ParseMessageKey(key); err == nil {
		seqStr := strings.TrimLeft(parts.Seq, "0")
		if seqStr == "" {
			return 0
		}
		if seq, err := strconv.ParseUint(seqStr, 10, 64); err == nil {
			return seq
		}
	}
	return 0
}
