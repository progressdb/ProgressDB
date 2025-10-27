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

// ResolveMessageKey is the unified method for handling message key resolution
// Takes a message key (provisional or final) and returns the final message key with sequence
// If the message key is provisional and new, it generates a sequenced final key
func (m *MessageSequencer) ResolveMessageKey(msgKey string, finalKeyIfNew string) (string, error) {
	if msgKey == "" {
		return "", fmt.Errorf("msgKey cannot be empty")
	}
	// If it's already a final key (has sequence), return as-is
	if !keys.IsProvisionalMessageKey(msgKey) {
		logger.Debug("resolve_final_key", "msg_key", msgKey)
		return msgKey, nil
	}

	// For provisional keys, check cache first
	if finalKey, ok := m.provisionalToFinalKeys[msgKey]; ok {
		logger.Debug("resolve_cache_hit", "provisional", msgKey, "final", finalKey)
		return finalKey, nil
	}

	// Not in cache - check database
	if storedb.Client == nil {
		logger.Debug("resolve_store_not_ready", "provisional", msgKey, "generating_new")
		return m.generateNewSequencedKey(msgKey, finalKeyIfNew)
	}

	// Create prefix for provisional key + ":" to find the sequenced key
	prefix := msgKey + ":"

	iter, err := storedb.Client.NewIter(nil)
	if err != nil {
		logger.Error("resolve_iterator_failed", "error", err, "provisional", msgKey, "generating_new")
		return m.generateNewSequencedKey(msgKey, finalKeyIfNew)
	}
	defer iter.Close()

	// Seek to the prefix
	iter.SeekGE([]byte(prefix))

	if iter.Valid() && len(iter.Key()) > len(prefix) && string(iter.Key()[:len(prefix)]) == prefix {
		// Found existing final key in database
		existingFinalKey := string(iter.Key())
		logger.Debug("resolve_db_found", "provisional", msgKey, "existing_final", existingFinalKey)

		// Cache it for future use
		m.provisionalToFinalKeys[msgKey] = existingFinalKey
		return existingFinalKey, nil
	}

	// Not found in database - this is a new provisional key
	logger.Debug("resolve_db_not_found", "provisional", msgKey, "generating_new")
	return m.generateNewSequencedKey(msgKey, finalKeyIfNew)
}

// generateNewSequencedKey creates a new sequenced key for a provisional message
func (m *MessageSequencer) generateNewSequencedKey(provisionalKey, finalKeyIfNew string) (string, error) {
	// Parse the thread key from the provisional key to get sequence
	threadKey, err := m.extractThreadKeyFromKey(provisionalKey)
	if err != nil {
		logger.Error("extract_thread_id_failed", "error", err, "provisional", provisionalKey)
		m.MapProvisionalToFinalMessageKey(provisionalKey, finalKeyIfNew)
		return finalKeyIfNew, nil
	}

	// Get next sequence atomically
	sequence := m.indexManager.GetNextThreadSequence(threadKey)

	// Extract message key from finalKeyIfNew or provisionalKey
	messageKey := m.extractMessageKeyFromKey(finalKeyIfNew)
	if messageKey == "" {
		messageKey = m.extractMessageKeyFromKey(provisionalKey)
	}

	// Generate the final sequenced key
	finalKey := keys.GenMessageKey(threadKey, messageKey, sequence)

	// Cache the mapping
	m.MapProvisionalToFinalMessageKey(provisionalKey, finalKey)

	logger.Debug("generated_sequenced_key", "provisional", provisionalKey, "final", finalKey, "sequence", sequence)
	return finalKey, nil
}

// extractThreadKeyFromKey extracts thread key from a provisional message key
func (m *MessageSequencer) extractThreadKeyFromKey(key string) (string, error) {
	if parts, err := keys.ParseThreadKey(key); err == nil {
		return parts.ThreadID, nil
	}

	// Try parsing as message key
	if parts, err := keys.ParseMessageKey(key); err == nil {
		return parts.ThreadID, nil
	}

	return "", fmt.Errorf("unable to extract thread key from key: %s", key)
}

// extractMessageKeyFromKey extracts message key from a key
func (m *MessageSequencer) extractMessageKeyFromKey(key string) string {
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
