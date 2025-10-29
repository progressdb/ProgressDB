package apply

import (
	"errors"
	"sync"

	"progressdb/pkg/state"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/index"

	"github.com/cockroachdb/pebble"
)

type IndexManager struct {
	mu               sync.RWMutex
	threadMessages   map[string]*index.ThreadMessageIndexes
	indexData        map[string][]byte
	messageSequencer *MessageSequencer
}

func NewIndexManager() *IndexManager {
	im := &IndexManager{
		threadMessages: make(map[string]*index.ThreadMessageIndexes),
		indexData:      make(map[string][]byte),
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

func (im *IndexManager) UpdateThreadMessageIndexes(threadKey string, createdAt, updatedAt int64, isDelete bool, msgKey string) {
	if threadKey == "" {
		logger.Error("update_thread_indexes_failed", "error", "threadKey cannot be empty")
		return
	}
	im.mu.Lock()
	defer im.mu.Unlock()

	idx := im.threadMessages[threadKey]
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
		im.threadMessages[threadKey] = idx
	}

	if isDelete {
		idx.Skips = append(idx.Skips, msgKey)
	} else {
		createdDelta := createdAt - idx.LastCreatedAt
		updatedDelta := updatedAt - idx.LastUpdatedAt
		idx.Cdeltas = append(idx.Cdeltas, createdDelta)
		idx.Udeltas = append(idx.Udeltas, updatedDelta)

		if idx.LastCreatedAt == 0 || createdAt < idx.LastCreatedAt {
			idx.LastCreatedAt = createdAt
		}
		if updatedAt > idx.LastUpdatedAt {
			idx.LastUpdatedAt = updatedAt
		}
	}
}

func (im *IndexManager) GetNextThreadSequence(threadKey string) uint64 {
	im.mu.Lock()
	defer im.mu.Unlock()

	idx := im.threadMessages[threadKey]
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
		im.threadMessages[threadKey] = idx
	}

	sequence := idx.End
	idx.End++
	return sequence
}

func (im *IndexManager) InitializeThreadSequencesFromDB(threadKeys []string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	for _, threadKey := range threadKeys {
		threadIdx, err := index.GetThreadMessageIndexes(threadKey)
		if err != nil {
			if errors.Is(err, pebble.ErrNotFound) {
				// new thread - so recreate it
				im.threadMessages[threadKey] = &index.ThreadMessageIndexes{
					Start:         0,
					End:           0,
					Cdeltas:       []int64{},
					Udeltas:       []int64{},
					Skips:         []string{},
					LastCreatedAt: 0,
					LastUpdatedAt: 0,
				}
			} else {
				// unexpected fatal error: preserve index state integrity
				state.Crash("index_state_init_failed", err)
			}
		} else {
			im.threadMessages[threadKey] = &threadIdx
		}
	}
	return nil
}

func (im *IndexManager) GetThreadMessages() map[string]*index.ThreadMessageIndexes {
	im.mu.RLock()
	defer im.mu.RUnlock()

	result := make(map[string]*index.ThreadMessageIndexes)
	for k, v := range im.threadMessages {
		result[k] = v
	}
	return result
}

func (im *IndexManager) GetIndexData() map[string][]byte {
	im.mu.RLock()
	defer im.mu.RUnlock()

	result := make(map[string][]byte)
	for k, v := range im.indexData {
		result[k] = append([]byte(nil), v...)
	}
	return result
}

func (im *IndexManager) PrepopulateProvisionalCache(mappings map[string]string) {
	im.mu.Lock()
	defer im.mu.Unlock()

	for provKey, finalKey := range mappings {
		im.messageSequencer.provisionalToFinalKeys[provKey] = finalKey
		logger.Debug("prepopulated_cache", "provisional", provKey, "final", finalKey)
	}

	logger.Debug("provisional_cache_prepopulated", "mappings_count", len(mappings))
}

func (im *IndexManager) ResolveMessageKey(provisionalKey, fallbackKey string) (string, error) {
	return im.messageSequencer.ResolveMessageKey(provisionalKey, fallbackKey)
}

func (im *IndexManager) DeleteThreadMessageIndexes(threadID string) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.threadMessages[threadID] = nil
}

func (im *IndexManager) UpdateUserOwnership(userID, threadID string, owns bool) {
	if owns {
		if err := index.MarkUserOwnsThread(userID, threadID); err != nil {
			logger.Error("mark_user_owns_thread_failed", "user_id", userID, "thread_id", threadID, "error", err)
		}
	} else {
		if err := index.UnmarkUserOwnsThread(userID, threadID); err != nil {
			logger.Error("unmark_user_owns_thread_failed", "user_id", userID, "thread_id", threadID, "error", err)
		}
	}
}

func (im *IndexManager) UpdateThreadParticipants(threadID, userID string, participates bool) {
	if participates {
		if err := index.MarkThreadHasUser(threadID, userID); err != nil {
			logger.Error("mark_thread_has_user_failed", "thread_id", threadID, "user_id", userID, "error", err)
		}
	} else {
		if err := index.UnmarkThreadHasUser(threadID, userID); err != nil {
			logger.Error("unmark_thread_has_user_failed", "thread_id", threadID, "user_id", userID, "error", err)
		}
	}
}

func (im *IndexManager) UpdateSoftDeletedThreads(userID, threadID string, deleted bool) {
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

func (im *IndexManager) Reset() {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.threadMessages = make(map[string]*index.ThreadMessageIndexes)
	im.indexData = make(map[string][]byte)
	im.messageSequencer.Reset()
}
