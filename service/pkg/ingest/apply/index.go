package apply

import (
	"sync"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/db/index"
)

// IndexManager handles index-only concerns and ephemeral state management
// This file contains:
// - Thread message index management (UpdateThreadMessageIndexes, GetNextThreadSequence)
// - Thread sequence initialization (InitializeThreadSequencesFromDB)
// - Index data structures and state access
// - MessageSequencer integration

type IndexManager struct {
	mu               sync.RWMutex
	threadMessages   map[string]*index.ThreadMessageIndexes
	indexData        map[string][]byte
	messageSequencer *MessageSequencer
}

// NewIndexManager creates a new index manager with initialized state
func NewIndexManager() *IndexManager {
	im := &IndexManager{
		threadMessages: make(map[string]*index.ThreadMessageIndexes),
		indexData:      make(map[string][]byte),
	}
	im.messageSequencer = NewMessageSequencer(im)
	return im
}

// InitThreadMessageIndexes initializes thread message indexes for a thread
func (im *IndexManager) InitThreadMessageIndexes(threadID string) {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.threadMessages[threadID] = &index.ThreadMessageIndexes{
		Start:         0,
		End:           0,
		Cdeltas:       []int64{},
		Udeltas:       []int64{},
		Skips:         []string{},
		LastCreatedAt: 0,
		LastUpdatedAt: 0,
	}
}

// UpdateThreadMessageIndexes updates thread message indexes with new operation
func (im *IndexManager) UpdateThreadMessageIndexes(threadID string, createdAt, updatedAt int64, isDelete bool, msgKey string) {
	im.mu.Lock()
	defer im.mu.Unlock()

	idx := im.threadMessages[threadID]
	if idx == nil {
		idx = &index.ThreadMessageIndexes{
			Start:         0,
			End:           0,
			Cdeltas:       []int64{},
			Udeltas:       []int64{},
			Skips:         []string{},
			LastCreatedAt: 0,
			LastUpdatedAt: 0,
		}
		im.threadMessages[threadID] = idx
	}

	if isDelete {
		idx.Skips = append(idx.Skips, msgKey)
	} else {
		if idx.LastCreatedAt == 0 || createdAt < idx.LastCreatedAt {
			idx.LastCreatedAt = createdAt
		}
		if updatedAt > idx.LastUpdatedAt {
			idx.LastUpdatedAt = updatedAt
		}
		// Note: For creates, sequence is incremented in ResolveMessageID
		// For updates/reactions, we need to increment here
		// We can detect creates by checking if the msgKey has a sequence
		if extractSequenceFromKey(msgKey) == 0 {
			// This is an update/reaction, increment sequence
			idx.End++
		}
		idx.Cdeltas = append(idx.Cdeltas, 1)
		idx.Udeltas = append(idx.Udeltas, 1)
	}
}

// GetNextThreadSequence returns the next sequence number for a thread atomically
func (im *IndexManager) GetNextThreadSequence(threadID string) uint64 {
	im.mu.Lock()
	defer im.mu.Unlock()

	idx := im.threadMessages[threadID]
	if idx == nil {
		idx = &index.ThreadMessageIndexes{
			Start:         0,
			End:           0,
			Cdeltas:       []int64{},
			Udeltas:       []int64{},
			Skips:         []string{},
			LastCreatedAt: 0,
			LastUpdatedAt: 0,
		}
		im.threadMessages[threadID] = idx
	}

	idx.End++
	return idx.End
}

// InitializeThreadSequencesFromDB loads existing thread message indexes from database
func (im *IndexManager) InitializeThreadSequencesFromDB(threadIDs []string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	for _, threadID := range threadIDs {
		threadIdx, err := index.GetThreadMessageIndexes(threadID)
		if err != nil {
			logger.Debug("load_thread_index_failed", "thread_id", threadID, "error", err)
			// If not found, initialize with defaults
			im.threadMessages[threadID] = &index.ThreadMessageIndexes{
				Start:         0,
				End:           0,
				Cdeltas:       []int64{},
				Udeltas:       []int64{},
				Skips:         []string{},
				LastCreatedAt: 0,
				LastUpdatedAt: 0,
			}
		} else {
			im.threadMessages[threadID] = &threadIdx
		}
	}
	return nil
}

// GetThreadMessages returns current thread message indexes state
func (im *IndexManager) GetThreadMessages() map[string]*index.ThreadMessageIndexes {
	im.mu.RLock()
	defer im.mu.RUnlock()

	result := make(map[string]*index.ThreadMessageIndexes)
	for k, v := range im.threadMessages {
		result[k] = v
	}
	return result
}

// GetIndexData returns current index data state
func (im *IndexManager) GetIndexData() map[string][]byte {
	im.mu.RLock()
	defer im.mu.RUnlock()

	result := make(map[string][]byte)
	for k, v := range im.indexData {
		result[k] = append([]byte(nil), v...)
	}
	return result
}

// PrepopulateProvisionalCache loads existing provisional->final mappings into MessageSequencer cache
func (im *IndexManager) PrepopulateProvisionalCache(mappings map[string]string) {
	im.mu.Lock()
	defer im.mu.Unlock()

	for provKey, finalKey := range mappings {
		im.messageSequencer.provisionalToFinalKeys[provKey] = finalKey
		logger.Debug("prepopulated_cache", "provisional", provKey, "final", finalKey)
	}

	logger.Debug("provisional_cache_prepopulated", "mappings_count", len(mappings))
}

// ResolveMessageID resolves a provisional message ID through the sequencer
func (im *IndexManager) ResolveMessageID(provisionalID, fallbackID string) (string, error) {
	return im.messageSequencer.ResolveMessageID(provisionalID, fallbackID)
}

// DeleteThreadMessageIndexes deletes thread message indexes
func (im *IndexManager) DeleteThreadMessageIndexes(threadID string) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.threadMessages[threadID] = nil
}

// Reset clears all accumulated index changes
func (im *IndexManager) Reset() {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.threadMessages = make(map[string]*index.ThreadMessageIndexes)
	im.indexData = make(map[string][]byte)
	im.messageSequencer.Reset()
}
