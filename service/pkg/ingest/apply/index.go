package apply

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"progressdb/pkg/models"
	"progressdb/pkg/state"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

type IndexManager struct {
	kv               *KVManager
	messageSequencer *MessageSequencer
	mu               sync.Mutex
}

func NewIndexManager(kv *KVManager) *IndexManager {
	im := &IndexManager{
		kv: kv,
	}
	im.messageSequencer = NewMessageSequencer(im, kv)
	return im
}

// loads
func (im *IndexManager) loadThreadIndex(threadKey string) (*indexdb.ThreadMessageIndexes, error) {
	idx := &indexdb.ThreadMessageIndexes{}

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

func (im *IndexManager) saveThreadIndex(threadKey string, idx *indexdb.ThreadMessageIndexes) error {
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

// helpers
func (im *IndexManager) UpdateThreadMessageIndexes(threadKey string, message *models.Message) {
	if threadKey == "" {
		logger.Error("update_thread_indexes_failed", "error", "threadKey cannot be empty")
		return
	}

	idx, err := im.loadThreadIndex(threadKey)
	if err != nil {
		logger.Error("failed to load thread index", "error", err)
		return
	}

	msgKey := message.Key
	createdAt := message.CreatedTS
	updatedAt := message.UpdatedTS
	isDelete := message.Deleted

	// deletes mean not new
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
		if idx.LastUpdatedAt == 0 || updatedAt > idx.LastUpdatedAt {
			idx.LastUpdatedAt = updatedAt
		}
		// .End is manually updated later
	}

	if err := im.saveThreadIndex(threadKey, idx); err != nil {
		logger.Error("failed to save thread index", "error", err)
	}
}

func (im *IndexManager) InitializeThreadSequencesFromDB(threadKeys []string) error {
	for _, threadKey := range threadKeys {
		// Check if already in batch
		if _, ok := im.kv.GetIndexKV(keys.GenThreadMessageEnd(threadKey)); ok {
			continue
		}
		idx, err := indexdb.GetThreadMessageIndexData(threadKey)
		if err != nil {
			if errors.Is(err, pebble.ErrNotFound) {
				idx = indexdb.ThreadMessageIndexes{
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

func (im *IndexManager) PrepopulateProvisionalCache(mappings map[string]string) {
	for provKey, finalKey := range mappings {
		im.kv.SetStateKV(provKey, finalKey)
		logger.Debug("prepopulated_cache", "provisional", provKey, "final", finalKey)
	}

	logger.Debug("provisional_cache_prepopulated", "mappings_count", len(mappings))
}

func (im *IndexManager) GetNextMessageSequence(threadKey string) (uint64, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	idx, err := im.loadThreadIndex(threadKey)
	if err != nil {
		return 0, fmt.Errorf("load index failed: %w", err)
	}

	// The next available sequence is whatever End currently points to.
	sequence := idx.End

	// Reserve the next slot for future messages.
	idx.End = sequence + 1

	// Persist the updated index state.
	if err := im.saveThreadIndex(threadKey, idx); err != nil {
		return 0, fmt.Errorf("save index failed: %w", err)
	}

	return sequence, nil
}

func (m *IndexManager) generateNewSequencedKey(messageKey string) (string, error) {
	parsed, err := keys.ParseKey(messageKey)
	if err != nil {
		return "", fmt.Errorf("invalid message key format: %w", err)
	}

	if parsed.Type != keys.KeyTypeMessage && parsed.Type != keys.KeyTypeMessageProvisional {
		return "", fmt.Errorf("expected message key, got %s", parsed.Type)
	}

	threadKey := keys.GenThreadKey(parsed.ThreadTS)

	// Get the next available sequence number for this thread
	sequence, err := m.GetNextMessageSequence(threadKey)
	if err != nil {
		return "", fmt.Errorf("get next message sequence: %w", err)
	}

	finalKey := keys.GenMessageKey(threadKey, parsed.MessageTS, sequence)

	// set state
	m.kv.SetStateKV(messageKey, finalKey)

	return finalKey, nil
}

func (im *IndexManager) ResolveMessageKey(msgKey string) (string, error) {
	// Check if msgKey is empty
	if msgKey == "" {
		logger.Debug("resolve_message_key", "source", "error", "msg", "msgKey cannot be empty")
		return "", fmt.Errorf("msgKey cannot be empty")
	}

	// Already a final (non-provisional) key? Just return it.
	if parsed, err := keys.ParseKey(msgKey); err == nil && parsed.Type != keys.KeyTypeMessageProvisional {
		logger.Debug("resolve_message_key", "source", "final_key_direct", "msgKey", msgKey)
		return msgKey, nil
	}

	// Check for an in-batch mapping for this provisional key
	if finalKey, ok := im.kv.GetStateKV(msgKey); ok {
		logger.Debug("resolve_message_key", "source", "batch_mapping", "msgKey", msgKey, "finalKey", finalKey)
		return finalKey, nil
	}

	// Try to resolve from DBs
	if finalKey, found := im.messageSequencer.resolveMessageFinalKeyFromDB(msgKey); found {
		im.kv.SetStateKV(msgKey, finalKey) // cache for batch
		logger.Debug("resolve_message_key", "source", "db_lookup", "msgKey", msgKey, "finalKey", finalKey)
		return finalKey, nil
	}

	// Otherwise, generate a new sequenced final key
	newFinalKey, err := im.generateNewSequencedKey(msgKey)
	if err == nil {
		logger.Debug("resolve_message_key", "source", "generated_new", "msgKey", msgKey, "finalKey", newFinalKey)
	}
	return newFinalKey, err
}

// relations
func (im *IndexManager) SetUserOwnership(userID, threadKey string, value int) {
	key := keys.GenUserOwnsThreadKey(userID, threadKey)
	im.kv.SetIndexKV(key, []byte(strconv.Itoa(value)))
}

func (im *IndexManager) SetThreadParticipants(userID, threadKey string, value int) {
	key := keys.GenThreadHasUserKey(threadKey, userID)
	im.kv.SetIndexKV(key, []byte(strconv.Itoa(value)))
}

// deletes
func (im *IndexManager) SetSoftDeletedThreads(userID, threadKey string, value int) {
	key := keys.GenSoftDeleteMarkerKey(threadKey)
	im.kv.SetIndexKV(key, []byte(strconv.Itoa(value)))
}

func (im *IndexManager) SetSoftDeletedMessages(userID, messageKey string, value int) {
	key := keys.GenSoftDeleteMarkerKey(messageKey)
	im.kv.SetIndexKV(key, []byte(strconv.Itoa(value)))
}

// Checks
func (im *IndexManager) DoesUserOwnThread(userID, threadKey string) (bool, error) {
	key := keys.GenUserOwnsThreadKey(userID, threadKey)
	if data, ok := im.kv.GetIndexKV(key); ok {
		return string(data) == "1", nil
	}
	// Not in batch, query DB
	return indexdb.DoesUserOwnThread(userID, threadKey)
}

func (im *IndexManager) DoesThreadHaveUser(threadKey, userID string) (bool, error) {
	key := keys.GenThreadHasUserKey(threadKey, userID)
	if data, ok := im.kv.GetIndexKV(key); ok {
		return string(data) == "1", nil
	}
	// Not in batch, query DB
	return indexdb.DoesThreadHaveUser(threadKey, userID)
}
