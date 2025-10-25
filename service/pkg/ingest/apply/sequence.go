package apply

import (
	"fmt"
	"strconv"
	"strings"

	"progressdb/pkg/logger"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
)

type MessageSequencer struct {
	provisionalToFinalKeys map[string]string
	batchIndexManager      *BatchIndexManager
}

func NewMessageSequencer(bim *BatchIndexManager) *MessageSequencer {
	return &MessageSequencer{
		provisionalToFinalKeys: make(map[string]string),
		batchIndexManager:      bim,
	}
}

func (m *MessageSequencer) MapProvisionalToFinalMessageKey(provisionalKey, finalKey string) {
	m.provisionalToFinalKeys[provisionalKey] = finalKey
	logger.Debug("mapped_provisional_message", "provisional", provisionalKey, "final", finalKey)
}

func (m *MessageSequencer) IsProvisionalMessageKey(messageKey string) bool {
	return keys.IsProvisionalMessageKey(messageKey)
}

func (m *MessageSequencer) Reset() {
	m.provisionalToFinalKeys = make(map[string]string)
}

// Key resolution methods moved from index.go for better separation of concerns
func (m *MessageSequencer) GetFinalThreadKey(threadKey string) (string, error) {
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return "", fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", threadKey)
	}
	return threadKey, nil
}

func (m *MessageSequencer) MapProvisionalToFinalID(provisionalID, finalID string) {
	logger.Debug("mapped_provisional_thread", "provisional", provisionalID, "final", finalID)
}

// ResolveMessageID is the unified method for handling message ID resolution
// Takes a message ID (provisional or final) and returns the final message ID with sequence
// If the message ID is provisional and new, it generates a sequenced final key
func (m *MessageSequencer) ResolveMessageID(msgID string, finalKeyIfNew string) (string, error) {
	// If it's already a final key (has sequence), return as-is
	if !keys.IsProvisionalMessageKey(msgID) {
		logger.Debug("resolve_final_key", "msg_id", msgID)
		return msgID, nil
	}

	// For provisional keys, check cache first
	if finalKey, ok := m.provisionalToFinalKeys[msgID]; ok {
		logger.Debug("resolve_cache_hit", "provisional", msgID, "final", finalKey)
		return finalKey, nil
	}

	// Not in cache - check database
	if storedb.Client == nil {
		logger.Debug("resolve_store_not_ready", "provisional", msgID, "generating_new")
		return m.generateNewSequencedKey(msgID, finalKeyIfNew)
	}

	// Create prefix for provisional key + ":" to find the sequenced key
	prefix := msgID + ":"

	iter, err := storedb.Client.NewIter(nil)
	if err != nil {
		logger.Error("resolve_iterator_failed", "error", err, "provisional", msgID, "generating_new")
		return m.generateNewSequencedKey(msgID, finalKeyIfNew)
	}
	defer iter.Close()

	// Seek to the prefix
	iter.SeekGE([]byte(prefix))

	if iter.Valid() && len(iter.Key()) > len(prefix) && string(iter.Key()[:len(prefix)]) == prefix {
		// Found existing final key in database
		existingFinalKey := string(iter.Key())
		logger.Debug("resolve_db_found", "provisional", msgID, "existing_final", existingFinalKey)

		// Cache it for future use
		m.provisionalToFinalKeys[msgID] = existingFinalKey
		return existingFinalKey, nil
	}

	// Not found in database - this is a new provisional key
	logger.Debug("resolve_db_not_found", "provisional", msgID, "generating_new")
	return m.generateNewSequencedKey(msgID, finalKeyIfNew)
}

// generateNewSequencedKey creates a new sequenced key for a provisional message
func (m *MessageSequencer) generateNewSequencedKey(provisionalKey, finalKeyIfNew string) (string, error) {
	// Parse the thread ID from the provisional key to get sequence
	threadID, err := m.extractThreadIDFromKey(provisionalKey)
	if err != nil {
		logger.Error("extract_thread_id_failed", "error", err, "provisional", provisionalKey)
		m.MapProvisionalToFinalMessageKey(provisionalKey, finalKeyIfNew)
		return finalKeyIfNew, nil
	}

	// Get next sequence atomically
	sequence := m.batchIndexManager.GetNextThreadSequence(threadID)

	// Extract message ID from finalKeyIfNew or provisionalKey
	messageID := m.extractMessageIDFromKey(finalKeyIfNew)
	if messageID == "" {
		messageID = m.extractMessageIDFromKey(provisionalKey)
	}

	// Generate the final sequenced key
	finalKey := keys.GenMessageKey(threadID, messageID, sequence)

	// Cache the mapping
	m.MapProvisionalToFinalMessageKey(provisionalKey, finalKey)

	logger.Debug("generated_sequenced_key", "provisional", provisionalKey, "final", finalKey, "sequence", sequence)
	return finalKey, nil
}

// extractThreadIDFromKey extracts thread ID from a provisional message key
func (m *MessageSequencer) extractThreadIDFromKey(key string) (string, error) {
	if parts, err := keys.ParseThreadKey(key); err == nil {
		return parts.ThreadID, nil
	}

	// Try parsing as message key
	if parts, err := keys.ParseMessageKey(key); err == nil {
		return parts.ThreadID, nil
	}

	return "", fmt.Errorf("unable to extract thread ID from key: %s", key)
}

// extractMessageIDFromKey extracts message ID from a key
func (m *MessageSequencer) extractMessageIDFromKey(key string) string {
	if parts, err := keys.ParseMessageKey(key); err == nil {
		return parts.MsgID
	}
	return ""
}

// extractSequenceFromKey extracts sequence number from a final message key
func extractSequenceFromKey(key string) uint64 {
	if parts, err := keys.ParseMessageKey(key); err == nil {
		// Simple conversion from padded string to uint64
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
