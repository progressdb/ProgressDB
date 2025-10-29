package apply

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"progressdb/pkg/state"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/index"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

type IndexManager struct {
	kv               *KVManager
	messageSequencer *MessageSequencer
}

func NewIndexManager(kv *KVManager) *IndexManager {
	return &IndexManager{
		kv: kv,
	}
}

func (im *IndexManager) loadThreadIndex(threadKey string) (*index.ThreadMessageIndexes, error) {
	idx := &index.ThreadMessageIndexes{}

	// Load Start
	if data, ok := im.kv.GetIndexKV(keys.GenThreadMessageStart(threadKey)); ok {
		if val, err := strconv.ParseInt(string(data), 10, 64); err == nil {
			idx.Start = uint64(val)
		}
	}

	// Load End
	if data, ok := im.kv.GetIndexKV(keys.GenThreadMessageEnd(threadKey)); ok {
		if val, err := strconv.ParseInt(string(data), 10, 64); err == nil {
			idx.End = uint64(val)
		}
	}

	// Load Cdeltas
	if data, ok := im.kv.GetIndexKV(keys.GenThreadMessageCDeltas(threadKey)); ok {
		if err := json.Unmarshal(data, &idx.Cdeltas); err != nil {
			logger.Error("failed to unmarshal cdeltas", "error", err)
		}
	}

	// Load Udeltas
	if data, ok := im.kv.GetIndexKV(keys.GenThreadMessageUDeltas(threadKey)); ok {
		if err := json.Unmarshal(data, &idx.Udeltas); err != nil {
			logger.Error("failed to unmarshal udeltas", "error", err)
		}
	}

	// Load Skips
	if data, ok := im.kv.GetIndexKV(keys.GenThreadMessageSkips(threadKey)); ok {
		if err := json.Unmarshal(data, &idx.Skips); err != nil {
			logger.Error("failed to unmarshal skips", "error", err)
		}
	}

	// Load LastCreatedAt
	if data, ok := im.kv.GetIndexKV(keys.GenThreadMessageLC(threadKey)); ok {
		if val, err := strconv.ParseInt(string(data), 10, 64); err == nil {
			idx.LastCreatedAt = val
		}
	}

	// Load LastUpdatedAt
	if data, ok := im.kv.GetIndexKV(keys.GenThreadMessageLU(threadKey)); ok {
		if val, err := strconv.ParseInt(string(data), 10, 64); err == nil {
			idx.LastUpdatedAt = val
		}
	}

	return idx, nil
}

func (im *IndexManager) saveThreadIndex(threadKey string, idx *index.ThreadMessageIndexes) error {
	// Save Start
	im.kv.SetIndexKV(keys.GenThreadMessageStart(threadKey), []byte(fmt.Sprintf("%d", idx.Start)))

	// Save End
	im.kv.SetIndexKV(keys.GenThreadMessageEnd(threadKey), []byte(fmt.Sprintf("%d", idx.End)))

	// Save Cdeltas
	if data, err := json.Marshal(idx.Cdeltas); err == nil {
		im.kv.SetIndexKV(keys.GenThreadMessageCDeltas(threadKey), data)
	}

	// Save Udeltas
	if data, err := json.Marshal(idx.Udeltas); err == nil {
		im.kv.SetIndexKV(keys.GenThreadMessageUDeltas(threadKey), data)
	}

	// Save Skips
	if data, err := json.Marshal(idx.Skips); err == nil {
		im.kv.SetIndexKV(keys.GenThreadMessageSkips(threadKey), data)
	}

	// Save LastCreatedAt
	im.kv.SetIndexKV(keys.GenThreadMessageLC(threadKey), []byte(fmt.Sprintf("%d", idx.LastCreatedAt)))

	// Save LastUpdatedAt
	im.kv.SetIndexKV(keys.GenThreadMessageLU(threadKey), []byte(fmt.Sprintf("%d", idx.LastUpdatedAt)))

	return nil
}

func (im *IndexManager) UpdateThreadMessageIndexes(threadKey string, createdAt, updatedAt int64, isDelete bool, msgKey string) {
	if threadKey == "" {
		logger.Error("update_thread_indexes_failed", "error", "threadKey cannot be empty")
		return
	}

	idx, err := im.loadThreadIndex(threadKey)
	if err != nil {
		logger.Error("failed to load thread index", "error", err)
		return
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

	if err := im.saveThreadIndex(threadKey, idx); err != nil {
		logger.Error("failed to save thread index", "error", err)
	}
}

func (im *IndexManager) GetNextThreadSequence(threadKey string) uint64 {
	idx, err := im.loadThreadIndex(threadKey)
	if err != nil {
		logger.Error("failed to load thread index", "error", err)
		return 0
	}

	sequence := idx.End
	idx.End++

	if err := im.saveThreadIndex(threadKey, idx); err != nil {
		logger.Error("failed to save thread index", "error", err)
		return sequence
	}

	return sequence
}

func (im *IndexManager) InitializeThreadSequencesFromDB(threadKeys []string) error {
	for _, threadKey := range threadKeys {
		// Check if already in batch
		if _, ok := im.kv.GetIndexKV(keys.GenThreadMessageEnd(threadKey)); ok {
			continue
		}
		idx, err := index.GetThreadMessageIndexes(threadKey)
		if err != nil {
			if errors.Is(err, pebble.ErrNotFound) {
				idx = index.ThreadMessageIndexes{
					Start:         0,
					End:           0,
					Cdeltas:       []int64{},
					Udeltas:       []int64{},
					Skips:         []string{},
					LastCreatedAt: 0,
					LastUpdatedAt: 0,
				}
			} else {
				state.Crash("index_state_init_failed", err)
			}
		}
		if err := im.saveThreadIndex(threadKey, &idx); err != nil {
			return err
		}
	}
	return nil
}

func (im *IndexManager) GetThreadMessages() map[string]*index.ThreadMessageIndexes {
	// Since indexes are stored in kv, but to collect all, we would need to iterate kv, but kv is private.
	// For now, return empty, as this method may not be used.
	result := make(map[string]*index.ThreadMessageIndexes)
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
	// To delete, set to nil
	im.kv.SetIndexKV(keys.GenThreadMessageStart(threadID), nil)
	im.kv.SetIndexKV(keys.GenThreadMessageEnd(threadID), nil)
	im.kv.SetIndexKV(keys.GenThreadMessageCDeltas(threadID), nil)
	im.kv.SetIndexKV(keys.GenThreadMessageUDeltas(threadID), nil)
	im.kv.SetIndexKV(keys.GenThreadMessageSkips(threadID), nil)
	im.kv.SetIndexKV(keys.GenThreadMessageLC(threadID), nil)
	im.kv.SetIndexKV(keys.GenThreadMessageLU(threadID), nil)
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
	// KV is reset in KVManager
}
