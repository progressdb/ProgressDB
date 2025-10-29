package apply

import (
	"fmt"
	"strconv"
	"strings"

	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
)

type MessageSequencer struct {
	kv           *KVManager
	indexManager *IndexManager
}

func NewMessageSequencer(im *IndexManager, kv *KVManager) *MessageSequencer {
	return &MessageSequencer{
		kv:           kv,
		indexManager: im,
	}
}

func (m *MessageSequencer) GetFinalThreadKey(threadKey string) (string, error) {
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return "", fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", threadKey)
	}
	return threadKey, nil
}

func (m *MessageSequencer) ResolveMessageKey(msgKey string) (string, error) {
	// Check if msgKey is empty
	if msgKey == "" {
		return "", fmt.Errorf("msgKey cannot be empty")
	}

	// Already a final (non-provisional) key? Just return it.
	if !keys.IsProvisionalMessageKey(msgKey) {
		return msgKey, nil
	}

	// Check for an in-batch mapping for this provisional key
	if finalKey, ok := m.kv.GetStateKV(msgKey); ok {
		return finalKey, nil
	}

	// Try to resolve from DBs
	if finalKey, found := m.resolveMessageKeyFromDB(msgKey); found {
		m.kv.SetStateKV(msgKey, finalKey) // cache for batch
		return finalKey, nil
	}

	// Otherwise, generate a new sequenced final key
	return m.generateNewSequencedKey(msgKey)
}

func (m *MessageSequencer) resolveMessageKeyFromDB(msgKey string) (string, bool) {
	if storedb.Client == nil {
		return "", false
	}
	prefix := msgKey + ":"

	iter, err := storedb.Client.NewIter(nil)
	if err != nil {
		return "", false
	}
	defer iter.Close()

	iter.SeekGE([]byte(prefix))
	if iter.Valid() && len(iter.Key()) > len(prefix) && string(iter.Key()[:len(prefix)]) == prefix {
		return string(iter.Key()), true
	}
	return "", false
}

func (m *MessageSequencer) generateNewSequencedKey(messageKey string) (string, error) {
	threadKey, err := m.extractThreadTs(messageKey)
	if err != nil {
		return "", err
	}

	sequence, err := m.indexManager.GetNextMessageSequence(threadKey)
	if err != nil {
		return "", err
	}

	parts, err := keys.ParseMessageKey(messageKey)
	if err != nil {
		return "", err
	}

	finalKey := keys.GenMessageKey(threadKey, parts.MsgID, sequence)

	// set state
	m.kv.SetStateKV(messageKey, finalKey)

	return finalKey, nil
}

func (m *MessageSequencer) extractThreadTs(key string) (string, error) {
	if parts, err := keys.ParseThreadKey(key); err == nil {
		return parts.ThreadID, nil
	}

	return "", fmt.Errorf("unable to extract thread key from key: %s", key)
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
