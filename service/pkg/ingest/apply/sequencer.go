package apply

import (
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/keys"
)

// ThreadSequencer manages thread ID resolution and sequencing
// With removal of :meta suffix, provisional and final keys are identical
type ThreadSequencer struct {
	// No complex mapping needed since provisional == final
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
	return &ThreadSequencer{}
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
// With removal of :meta suffix, this is a no-op since provisional == final
func (t *ThreadSequencer) MapProvisionalToFinalThreadID(provisionalID, finalID string) {
	logger.Debug("mapped_provisional_thread", "provisional", provisionalID, "final", finalID)
	// No mapping needed since provisional == final
}

// Resolves a provisional or final thread ID to the final thread ID
// With the removal of :meta suffix, provisional and final keys are identical
func (t *ThreadSequencer) GetFinalThreadID(threadID string) (string, error) {
	// Both provisional and final thread keys have the same format: t:<threadID>
	if keys.ValidateThreadKey(threadID) != nil && keys.ValidateThreadPrvKey(threadID) != nil {
		return "", fmt.Errorf("invalid thread ID format: %s - expected t:<threadID>", threadID)
	}

	// Return the key as-is since provisional and final are now identical
	return threadID, nil
}

// Maps a provisional message ID to its final ID for this batch
func (m *MessageSequencer) MapProvisionalToFinalMessageID(provisionalID, finalID string) {
	m.provisionalToFinalIDs[provisionalID] = finalID
	logger.Debug("mapped_provisional_message", "provisional", provisionalID, "final", finalID)
}

// Resolves a provisional or final message ID to the final message ID
func (m *MessageSequencer) GetFinalMessageID(messageID string) (string, error) {
	// If it's a provisional message ID (raw ID), return as is
	if keys.IsProvisionalMessageID(messageID) {
		return messageID, nil
	}

	// Batch-local mapping
	if finalID, ok := m.provisionalToFinalIDs[messageID]; ok {
		return finalID, nil
	}

	// Cached resolution
	if finalID, ok := m.resolvedMessageIDs[messageID]; ok {
		return finalID, nil
	}

	// Not resolved - return as is
	logger.Debug("message_id_not_resolved", "provisional", messageID)
	return messageID, nil
}

// Returns true if the message ID is in provisional format
func (m *MessageSequencer) IsProvisionalMessageID(messageID string) bool {
	return keys.IsProvisionalMessageID(messageID)
}

// Resets all cached mappings in thread sequencer
func (t *ThreadSequencer) Reset() {
	// No cached mappings needed since provisional == final
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
