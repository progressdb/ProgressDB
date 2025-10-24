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
	provisionalToFinalIDs map[string]string // provisional -> final, for this batch
	resolvedThreadIDs     map[string]string // cache: provisional -> final, via DB
}

// MessageSequencer manages message ID resolution and sequencing
type MessageSequencer struct {
	provisionalToFinalIDs map[string]string // provisional -> final, for this batch
	resolvedMessageIDs    map[string]string // cache: provisional -> final, via DB
}

// BatchSequencerManager manages thread and message sequencers for batch processing
type BatchSequencerManager struct {
	threadSequencer  *ThreadSequencer
	messageSequencer *MessageSequencer
}

func NewThreadSequencer() *ThreadSequencer {
	return &ThreadSequencer{
		provisionalToFinalIDs: make(map[string]string),
		resolvedThreadIDs:     make(map[string]string),
	}
}

func NewMessageSequencer() *MessageSequencer {
	return &MessageSequencer{
		provisionalToFinalIDs: make(map[string]string),
		resolvedMessageIDs:    make(map[string]string),
	}
}

func NewBatchSequencerManager() *BatchSequencerManager {
	return &BatchSequencerManager{
		threadSequencer:  NewThreadSequencer(),
		messageSequencer: NewMessageSequencer(),
	}
}

// Maps a provisional thread ID to its final ID for this batch
func (t *ThreadSequencer) MapProvisionalToFinalThreadID(provisionalID, finalID string) {
	t.provisionalToFinalIDs[provisionalID] = finalID
	logger.Debug("mapped_provisional_thread", "provisional", provisionalID, "final", finalID)
}

// Resolves a provisional or final thread ID to the final thread ID
func (t *ThreadSequencer) GetFinalThreadID(threadID string) (string, error) {
	// Check if it's a final thread key format: t:<threadID>:meta
	if keys.ValidateThreadKey(threadID) == nil {
		// Extract thread ID from final key format
		parts, err := keys.ParseThreadMeta(threadID)
		if err != nil {
			return "", fmt.Errorf("failed to parse valid thread key %s: %w", threadID, err)
		}
		return parts.ThreadID, nil
	}

	// Check if it's a provisional thread key format: t:<threadID>
	if keys.ValidateThreadPrvKey(threadID) == nil {
		// Extract provisional thread ID from provisional key format
		provisionalThreadID := threadID[2:] // Remove "t:" prefix

		// Batch-local mapping
		if finalID, ok := t.provisionalToFinalIDs[provisionalThreadID]; ok {
			return finalID, nil
		}

		// Cached from DB
		if finalID, ok := t.resolvedThreadIDs[provisionalThreadID]; ok {
			return finalID, nil
		}

		// DB resolution
		finalID, err := t.resolveThreadIDFromDB(provisionalThreadID)
		if err != nil {
			return "", err
		}
		t.resolvedThreadIDs[provisionalThreadID] = finalID
		return finalID, nil
	}

	// Invalid key format - this should never happen
	return "", fmt.Errorf("invalid thread ID format: %s - expected either provisional (t:<threadID>) or final (t:<threadID>:meta) format", threadID)
}

// Looks up the final thread ID for a provisional one in the DB
func (t *ThreadSequencer) resolveThreadIDFromDB(provisionalID string) (string, error) {
	timestamp, err := keys.ParseProvisionalThreadID(provisionalID)
	if err != nil {
		return "", fmt.Errorf("invalid provisional ID format: %w", err)
	}
	threadID := keys.GenThreadIDFromTimestamp(timestamp)
	threadKey := keys.GenThreadKey(threadID)

	iter, err := storedb.Client.NewIter(&pebble.IterOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	iter.SeekGE([]byte(threadKey))

	if iter.Valid() {
		key := iter.Key()
		if string(key) == threadKey {
			parts, err := keys.ParseThreadMeta(string(key))
			if err != nil {
				return "", fmt.Errorf("failed to parse thread meta key: %w", err)
			}
			finalThreadID := parts.ThreadID
			logger.Debug("resolved_provisional_id", "provisional", provisionalID, "final", finalThreadID)
			return finalThreadID, nil
		}
	}

	logger.Error("provisional_thread_not_found", "provisional", provisionalID, "timestamp", timestamp)
	return "", fmt.Errorf("thread with provisional ID %s (timestamp %d) not found in database", provisionalID, timestamp)
}

// Maps a provisional message ID to its final ID for this batch
func (m *MessageSequencer) MapProvisionalToFinalMessageID(provisionalID, finalID string) {
	m.provisionalToFinalIDs[provisionalID] = finalID
	logger.Debug("mapped_provisional_message", "provisional", provisionalID, "final", finalID)
}

// Resolves a provisional or final message ID to the final message ID
func (m *MessageSequencer) GetFinalMessageID(messageID string) (string, error) {
	// If already a final message ID, return as is
	if !strings.HasPrefix(messageID, "msg-") {
		return messageID, nil
	}
	// Batch-local
	if finalID, ok := m.provisionalToFinalIDs[messageID]; ok {
		return finalID, nil
	}
	// Cached resolution
	if finalID, ok := m.resolvedMessageIDs[messageID]; ok {
		return finalID, nil
	}
	// Not resolved
	logger.Debug("message_id_not_resolved", "provisional", messageID)
	return messageID, nil
}

// Returns true if the message ID is in provisional format
func (m *MessageSequencer) IsProvisionalMessageID(messageID string) bool {
	return strings.HasPrefix(messageID, "msg-")
}

// Resets all cached mappings in thread sequencer
func (t *ThreadSequencer) Reset() {
	t.provisionalToFinalIDs = make(map[string]string)
	t.resolvedThreadIDs = make(map[string]string)
}

// Resets all cached mappings in message sequencer
func (m *MessageSequencer) Reset() {
	m.provisionalToFinalIDs = make(map[string]string)
	m.resolvedMessageIDs = make(map[string]string)
}

// BatchSequencerManager helpers

func (bsm *BatchSequencerManager) MapProvisionalToFinalThreadID(provisionalID, finalID string) {
	bsm.threadSequencer.MapProvisionalToFinalThreadID(provisionalID, finalID)
}

func (bsm *BatchSequencerManager) GetFinalThreadID(threadID string) (string, error) {
	return bsm.threadSequencer.GetFinalThreadID(threadID)
}

func (bsm *BatchSequencerManager) MapProvisionalToFinalMessageID(provisionalID, finalID string) {
	bsm.messageSequencer.MapProvisionalToFinalMessageID(provisionalID, finalID)
}

func (bsm *BatchSequencerManager) GetFinalMessageID(messageID string) (string, error) {
	return bsm.messageSequencer.GetFinalMessageID(messageID)
}

func (bsm *BatchSequencerManager) IsProvisionalMessageID(messageID string) bool {
	return bsm.messageSequencer.IsProvisionalMessageID(messageID)
}

// Resets all sequencer state
func (bsm *BatchSequencerManager) Reset() {
	bsm.threadSequencer.Reset()
	bsm.messageSequencer.Reset()
}
