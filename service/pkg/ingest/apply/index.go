package apply

import (
	"sync"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/db/index"
)

type IndexManager struct {
	mu                 sync.RWMutex
	threadMessages     map[string]*index.ThreadMessageIndexes
	indexData          map[string][]byte
	messageSequencer   *MessageSequencer
	userOwnership      map[string]map[string]bool // userID -> threadID -> owns
	threadParticipants map[string]map[string]bool // threadID -> userID -> participates
}

func NewIndexManager() *IndexManager {
	im := &IndexManager{
		threadMessages:     make(map[string]*index.ThreadMessageIndexes),
		indexData:          make(map[string][]byte),
		userOwnership:      make(map[string]map[string]bool),
		threadParticipants: make(map[string]map[string]bool),
	}
	im.messageSequencer = NewMessageSequencer(im)
	return im
}

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
		// Note: Sequence is only incremented in ResolveMessageID for new messages
		// Updates and reactions should not increment the sequence
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

// InitializeUserOwnershipFromDB loads user ownership data for specific users
func (im *IndexManager) InitializeUserOwnershipFromDB(userIDs []string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	for _, userID := range userIDs {
		threads, err := index.GetUserThreads(userID)
		if err != nil {
			logger.Debug("load_user_ownership_failed", "user_id", userID, "error", err)
			continue
		}

		if im.userOwnership[userID] == nil {
			im.userOwnership[userID] = make(map[string]bool)
		}

		for _, threadID := range threads {
			im.userOwnership[userID][threadID] = true
		}
	}
	return nil
}

// InitializeThreadParticipantsFromDB loads thread participants data for specific threads
func (im *IndexManager) InitializeThreadParticipantsFromDB(threadIDs []string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	for _, threadID := range threadIDs {
		participants, err := index.GetThreadParticipants(threadID)
		if err != nil {
			logger.Debug("load_thread_participants_failed", "thread_id", threadID, "error", err)
			continue
		}

		if im.threadParticipants[threadID] == nil {
			im.threadParticipants[threadID] = make(map[string]bool)
		}

		for _, participantID := range participants {
			im.threadParticipants[threadID][participantID] = true
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

// User ownership tracking methods
func (im *IndexManager) UpdateUserOwnership(userID, threadID string, owns bool) {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.userOwnership[userID] == nil {
		im.userOwnership[userID] = make(map[string]bool)
	}
	im.userOwnership[userID][threadID] = owns
}

func (im *IndexManager) GetUserOwnership() map[string]map[string]bool {
	im.mu.RLock()
	defer im.mu.RUnlock()

	result := make(map[string]map[string]bool)
	for userID, threads := range im.userOwnership {
		result[userID] = make(map[string]bool)
		for threadID, owns := range threads {
			result[userID][threadID] = owns
		}
	}
	return result
}

// Thread participants tracking methods
func (im *IndexManager) UpdateThreadParticipants(threadID, userID string, participates bool) {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.threadParticipants[threadID] == nil {
		im.threadParticipants[threadID] = make(map[string]bool)
	}
	im.threadParticipants[threadID][userID] = participates
}

func (im *IndexManager) GetThreadParticipants() map[string]map[string]bool {
	im.mu.RLock()
	defer im.mu.RUnlock()

	result := make(map[string]map[string]bool)
	for threadID, users := range im.threadParticipants {
		result[threadID] = make(map[string]bool)
		for userID, participates := range users {
			result[threadID][userID] = participates
		}
	}
	return result
}

// Soft deleted tracking methods - now use key-based markers
func (im *IndexManager) UpdateSoftDeletedThreads(userID, threadID string, deleted bool) {
	// Update the key-based marker immediately
	if deleted {
		if err := index.MarkSoftDeleted(threadID); err != nil {
			logger.Error("mark_thread_soft_deleted_failed", "thread_id", threadID, "error", err)
		}
	} else {
		if err := index.UnmarkSoftDeleted(threadID); err != nil {
			logger.Error("unmark_thread_soft_deleted_failed", "thread_id", threadID, "error", err)
		}
	}
}

func (im *IndexManager) UpdateSoftDeletedMessages(userID, messageID string, deleted bool) {
	// Update the key-based marker immediately
	if deleted {
		if err := index.MarkSoftDeleted(messageID); err != nil {
			logger.Error("mark_message_soft_deleted_failed", "message_id", messageID, "error", err)
		}
	} else {
		if err := index.UnmarkSoftDeleted(messageID); err != nil {
			logger.Error("unmark_message_soft_deleted_failed", "message_id", messageID, "error", err)
		}
	}
}

// Reset clears all accumulated index changes
func (im *IndexManager) Reset() {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.threadMessages = make(map[string]*index.ThreadMessageIndexes)
	im.indexData = make(map[string][]byte)
	im.userOwnership = make(map[string]map[string]bool)
	im.threadParticipants = make(map[string]map[string]bool)
	im.messageSequencer.Reset()
}
