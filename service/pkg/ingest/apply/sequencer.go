package apply

import (
	"fmt"
	"strings"

	"progressdb/pkg/logger"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

// ThreadSequencer manages thread ID resolution and sequencing
type ThreadSequencer struct {
	provisionalToFinalIDs map[string]string // Maps provisional IDs to final IDs within this batch
	resolvedThreadIDs     map[string]string // Cache for database-resolved thread IDs
}

// MessageSequencer manages message ID resolution and sequencing
type MessageSequencer struct {
	provisionalToFinalIDs map[string]string // Maps provisional message IDs to final IDs within this batch
	resolvedMessageIDs    map[string]string // Cache for database-resolved message IDs
}

// BatchSequencerManager manages both thread and message sequencers for batch processing
type BatchSequencerManager struct {
	threadSequencer  *ThreadSequencer
	messageSequencer *MessageSequencer
}

// NewThreadSequencer creates a new thread sequencer
func NewThreadSequencer() *ThreadSequencer {
	return &ThreadSequencer{
		provisionalToFinalIDs: make(map[string]string),
		resolvedThreadIDs:     make(map[string]string),
	}
}

// NewMessageSequencer creates a new message sequencer
func NewMessageSequencer() *MessageSequencer {
	return &MessageSequencer{
		provisionalToFinalIDs: make(map[string]string),
		resolvedMessageIDs:    make(map[string]string),
	}
}

// NewBatchSequencerManager creates a new batch sequencer manager
func NewBatchSequencerManager() *BatchSequencerManager {
	return &BatchSequencerManager{
		threadSequencer:  NewThreadSequencer(),
		messageSequencer: NewMessageSequencer(),
	}
}

// MapProvisionalToFinalThreadID stores the mapping from provisional to final thread ID
func (t *ThreadSequencer) MapProvisionalToFinalThreadID(provisionalID, finalID string) {
	t.provisionalToFinalIDs[provisionalID] = finalID
	logger.Debug("mapped_provisional_thread", "provisional", provisionalID, "final", finalID)
}

// GetFinalThreadID resolves a provisional or final thread ID to the final ID
func (t *ThreadSequencer) GetFinalThreadID(threadID string) (string, error) {
	// If it's already a final ID, return it
	// For now, assume all thread IDs are final unless they look like provisional IDs
	if !strings.HasPrefix(threadID, "t:") && strings.Contains(threadID, "-") {
		return threadID, nil
	}

	// Check batch-local mapping first
	if finalID, exists := t.provisionalToFinalIDs[threadID]; exists {
		return finalID, nil
	}

	// Check resolution cache
	if finalID, exists := t.resolvedThreadIDs[threadID]; exists {
		return finalID, nil
	}

	// Need to resolve from database
	finalID, err := t.resolveThreadIDFromDB(threadID)
	if err != nil {
		return "", err
	}

	// Cache the result
	t.resolvedThreadIDs[threadID] = finalID
	return finalID, nil
}

// resolveThreadIDFromDB looks up the final thread ID from the database using provisional prefix
func (t *ThreadSequencer) resolveThreadIDFromDB(provisionalID string) (string, error) {
	// Extract timestamp from provisional ID - assume format "thread-{timestamp}"
	var timestamp int64
	if _, err := fmt.Sscanf(provisionalID, "thread-%d", &timestamp); err != nil {
		return "", fmt.Errorf("invalid provisional ID format: %w", err)
	}

	// Since timestamp is unique per request, we can directly construct the thread key prefix
	// and search for any thread that matches this timestamp
	threadPrefix := fmt.Sprintf("thread:thread-%d:", timestamp)

	// Use the store's prefix scanning capability to find the thread
	// This is much more efficient than iterating through possible sequences
	iter, err := storedb.Client.NewIter(&pebble.IterOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	// Seek to the prefix
	iter.SeekGE([]byte(threadPrefix))

	// Check if we found a thread with this timestamp prefix
	if iter.Valid() {
		key := iter.Key()
		if strings.HasPrefix(string(key), threadPrefix) {
			// Extract the full thread ID from the key
			finalThreadID := string(key)

			// Verify this is actually a thread metadata key
			expectedKey := keys.GenThreadKey(finalThreadID)
			if string(key) == expectedKey {
				logger.Debug("resolved_provisional_id", "provisional", provisionalID, "final", finalThreadID)
				return finalThreadID, nil
			}
		}
	}

	logger.Error("provisional_thread_not_found", "provisional", provisionalID, "timestamp", timestamp)
	return "", fmt.Errorf("thread with provisional ID %s (timestamp %d) not found in database", provisionalID, timestamp)
}

// MapProvisionalToFinalMessageID stores mapping from provisional to final message ID
func (m *MessageSequencer) MapProvisionalToFinalMessageID(provisionalID, finalID string) {
	m.provisionalToFinalIDs[provisionalID] = finalID
	logger.Debug("mapped_provisional_message", "provisional", provisionalID, "final", finalID)
}

// GetFinalMessageID resolves a provisional or final message ID to the final ID
func (m *MessageSequencer) GetFinalMessageID(messageID string) (string, error) {
	// If it's already a final ID, return it
	// For now, assume all message IDs are final unless they look like provisional IDs
	if !strings.HasPrefix(messageID, "msg-") {
		return messageID, nil
	}

	// Check batch-local mapping first
	if finalID, exists := m.provisionalToFinalIDs[messageID]; exists {
		return finalID, nil
	}

	// Check resolution cache
	if finalID, exists := m.resolvedMessageIDs[messageID]; exists {
		return finalID, nil
	}

	// For now, return the provisional ID if we can't resolve it
	// In the future, we might want to add database resolution logic here
	logger.Debug("message_id_not_resolved", "provisional", messageID)
	return messageID, nil
}

// IsProvisionalMessageID checks if a message ID is provisional
func (m *MessageSequencer) IsProvisionalMessageID(messageID string) bool {
	return strings.HasPrefix(messageID, "msg-")
}

// Reset clears all cached mappings
func (t *ThreadSequencer) Reset() {
	t.provisionalToFinalIDs = make(map[string]string)
	t.resolvedThreadIDs = make(map[string]string)
}

func (m *MessageSequencer) Reset() {
	m.provisionalToFinalIDs = make(map[string]string)
	m.resolvedMessageIDs = make(map[string]string)
}

// BatchSequencerManager methods

// MapProvisionalToFinalThreadID delegates to thread sequencer
func (bsm *BatchSequencerManager) MapProvisionalToFinalThreadID(provisionalID, finalID string) {
	bsm.threadSequencer.MapProvisionalToFinalThreadID(provisionalID, finalID)
}

// GetFinalThreadID delegates to thread sequencer
func (bsm *BatchSequencerManager) GetFinalThreadID(threadID string) (string, error) {
	return bsm.threadSequencer.GetFinalThreadID(threadID)
}

// MapProvisionalToFinalMessageID delegates to message sequencer
func (bsm *BatchSequencerManager) MapProvisionalToFinalMessageID(provisionalID, finalID string) {
	bsm.messageSequencer.MapProvisionalToFinalMessageID(provisionalID, finalID)
}

// GetFinalMessageID delegates to message sequencer
func (bsm *BatchSequencerManager) GetFinalMessageID(messageID string) (string, error) {
	return bsm.messageSequencer.GetFinalMessageID(messageID)
}

// IsProvisionalMessageID delegates to message sequencer
func (bsm *BatchSequencerManager) IsProvisionalMessageID(messageID string) bool {
	return bsm.messageSequencer.IsProvisionalMessageID(messageID)
}

// Reset clears all sequencer state
func (bsm *BatchSequencerManager) Reset() {
	bsm.threadSequencer.Reset()
	bsm.messageSequencer.Reset()
}
