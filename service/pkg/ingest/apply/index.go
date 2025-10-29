package apply

import (
	"errors"
	"sync"

	"progressdb/pkg/state"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/index"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

type IndexManager struct {
	kv               *KVManager
	mu               sync.RWMutex
	threadMessages   map[string]*index.ThreadMessageIndexes
	messageSequencer *MessageSequencer
}

func NewIndexManager(kv *KVManager) *IndexManager {
	im := &IndexManager{
		kv:             kv,
		threadMessages: make(map[string]*index.ThreadMessageIndexes),
	}
	im.messageSequencer = NewMessageSequencer(im, kv)
	return im
}

// func (im *IndexManager) InitThreadMessageIndexes(threadID string) {
// 	im.mu.Lock()
// 	defer im.mu.Unlock()

// 	im.threadMessages[threadID] = &index.ThreadMessageIndexes{
// 		Start:         0,
// 		End:           0,
// 		Cdeltas:       []int64{},
// 		Udeltas:       []int64{},
// 		Skips:         []string{},
// 		LastCreatedAt: 0,
// 		LastUpdatedAt: 0,
// 	}
// }

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

func (im *IndexManager) PrepopulateProvisionalCache(mappings map[string]string) {
	for provKey, finalKey := range mappings {
		im.kv.SetStateKV(provKey, finalKey)
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
	key := keys.GenUserOwnsThreadKey(userID, threadID)
	if owns {
		im.kv.SetIndexKV(key, []byte("1"))
	} else {
		im.kv.SetIndexKV(key, nil) // or delete, but since batch, set to nil?
	}
}

func (im *IndexManager) UpdateThreadParticipants(threadID, userID string, participates bool) {
	key := keys.GenThreadHasUserKey(threadID, userID)
	if participates {
		im.kv.SetIndexKV(key, []byte("1"))
	} else {
		im.kv.SetIndexKV(key, nil)
	}
}

func (im *IndexManager) UpdateSoftDeletedThreads(userID, threadID string, deleted bool) {
	key := keys.GenSoftDeleteMarkerKey(threadID)
	if deleted {
		im.kv.SetIndexKV(key, []byte("1"))
	} else {
		im.kv.SetIndexKV(key, nil)
	}
}

func (im *IndexManager) UpdateSoftDeletedMessages(userID, messageID string, deleted bool) {
	key := keys.GenSoftDeleteMarkerKey(messageID)
	if deleted {
		im.kv.SetIndexKV(key, []byte("1"))
	} else {
		im.kv.SetIndexKV(key, nil)
	}
}

func (im *IndexManager) Reset() {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.threadMessages = make(map[string]*index.ThreadMessageIndexes)
}
