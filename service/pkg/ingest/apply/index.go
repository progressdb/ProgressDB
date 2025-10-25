package apply

import (
	"encoding/json"
	"fmt"
	"sync"

	"progressdb/pkg/logger"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
)

type BatchIndexManager struct {
	mu               sync.RWMutex
	threadMessages   map[string]*index.ThreadMessageIndexes
	threadMeta       map[string][]byte
	messageData      map[string]MessageData
	indexData        map[string][]byte
	messageSequencer *MessageSequencer
}
type MessageData struct {
	Key  string
	Data []byte
	TS   int64
	Seq  uint64
}

func NewBatchIndexManager() *BatchIndexManager {
	return &BatchIndexManager{
		threadMessages:   make(map[string]*index.ThreadMessageIndexes),
		threadMeta:       make(map[string][]byte),
		messageData:      make(map[string]MessageData),
		indexData:        make(map[string][]byte),
		messageSequencer: NewMessageSequencer(),
	}
}

func (b *BatchIndexManager) InitThreadMessageIndexes(threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.threadMessages[threadID] = &index.ThreadMessageIndexes{
		Start:         0,
		End:           0,
		Cdeltas:       []int64{},
		Udeltas:       []int64{},
		Skips:         []string{},
		LastCreatedAt: 0,
		LastUpdatedAt: 0,
	}
}

func (b *BatchIndexManager) UpdateThreadMessageIndexes(threadID string, createdAt, updatedAt int64, isDelete bool, msgKey string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	idx := b.threadMessages[threadID]
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
		b.threadMessages[threadID] = idx
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
		idx.End++
		idx.Cdeltas = append(idx.Cdeltas, 1)
		idx.Udeltas = append(idx.Udeltas, 1)
	}
}

// InitializeThreadSequencesFromDB loads existing thread message indexes from database
func (b *BatchIndexManager) InitializeThreadSequencesFromDB(threadIDs []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, threadID := range threadIDs {
		threadIdx, err := index.GetThreadMessageIndexes(threadID)
		if err != nil {
			logger.Debug("load_thread_index_failed", "thread_id", threadID, "error", err)
			// If not found, initialize with defaults
			b.threadMessages[threadID] = &index.ThreadMessageIndexes{
				Start:         0,
				End:           0,
				Cdeltas:       []int64{},
				Udeltas:       []int64{},
				Skips:         []string{},
				LastCreatedAt: 0,
				LastUpdatedAt: 0,
			}
		} else {
			b.threadMessages[threadID] = &threadIdx
		}
	}
	return nil
}

// SetThreadMeta sets thread metadata
func (b *BatchIndexManager) SetThreadMeta(threadID string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.threadMeta[threadID] = data
}

// DeleteThreadMeta deletes thread metadata
func (b *BatchIndexManager) DeleteThreadMeta(threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.threadMeta[threadID] = nil
}

// DeleteThreadMessageIndexes deletes thread message indexes
func (b *BatchIndexManager) DeleteThreadMessageIndexes(threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.threadMessages[threadID] = nil
}

// SetMessageData sets message data
func (b *BatchIndexManager) SetMessageData(threadID, messageID string, data []byte, ts int64, seq uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	messageKey := keys.GenMessageKey(threadID, messageID, seq)
	b.messageData[messageKey] = MessageData{
		Key:  messageKey,
		Data: data,
		TS:   ts,
		Seq:  seq,
	}
	return nil
}

// AddMessageVersion adds a message version (now uses direct index call)
func (b *BatchIndexManager) AddMessageVersion(messageID string, data []byte, ts int64, seq uint64) error {
	versionKey := keys.GenVersionKey(messageID, ts, seq)
	return index.SaveKey(versionKey, data)
}

// AddThreadToUser adds thread to user (now uses direct index call)
func (b *BatchIndexManager) AddThreadToUser(userID, threadID string) error {
	return index.UpdateUserOwnership(userID, threadID, true)
}

// RemoveThreadFromUser removes thread from user (now uses direct index call)
func (b *BatchIndexManager) RemoveThreadFromUser(userID, threadID string) error {
	return index.UpdateUserOwnership(userID, threadID, false)
}

// AddParticipantToThread adds participant to thread (now uses direct index call)
func (b *BatchIndexManager) AddParticipantToThread(threadID, userID string) error {
	return index.UpdateThreadParticipants(threadID, userID, true)
}

// AddDeletedThreadToUser adds deleted thread to user (now uses direct index call)
func (b *BatchIndexManager) AddDeletedThreadToUser(userID, threadID string) error {
	return index.UpdateDeletedThreads(userID, threadID, true)
}

// AddDeletedMessageToUser adds deleted message to user (now uses direct index call)
func (b *BatchIndexManager) AddDeletedMessageToUser(userID, messageID string) error {
	return index.UpdateDeletedMessages(userID, messageID, true)
}

func (b *BatchIndexManager) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	logger.Debug("batch_flush_accumulated",
		"threads", len(b.threadMeta),
		"messages", len(b.messageData))

	var errors []error

	mainBatch := storedb.Client.NewBatch()
	indexBatch := index.IndexDB.NewBatch()

	for threadID, data := range b.threadMeta {
		threadKey := keys.GenThreadKey(threadID)

		if data == nil {
			if err := mainBatch.Delete([]byte(threadKey), storedb.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("delete thread meta %s: %w", threadID, err))
			}
		} else {
			if err := mainBatch.Set([]byte(threadKey), data, storedb.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set thread meta %s: %w", threadID, err))
			}
		}
	}

	for key, msgData := range b.messageData {
		if err := mainBatch.Set([]byte(key), msgData.Data, storedb.WriteOpt(true)); err != nil {
			errors = append(errors, fmt.Errorf("set message data %s: %w", key, err))
		}
	}

	for threadID, threadIdx := range b.threadMessages {
		threadKey := keys.GenThreadMessageStart(threadID)

		if threadIdx == nil {
			if err := indexBatch.Delete([]byte(threadKey), index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("delete thread messages %s: %w", threadID, err))
			}
		} else {
			data, err := json.Marshal(threadIdx)
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal thread messages %s: %w", threadID, err))
				continue
			}
			if err := indexBatch.Set([]byte(threadKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set thread messages %s: %w", threadID, err))
			}
		}
	}

	for key, data := range b.indexData {
		if err := indexBatch.Set([]byte(key), data, index.WriteOpt(true)); err != nil {
			errors = append(errors, fmt.Errorf("set index data %s: %w", key, err))
		}
	}

	if len(errors) == 0 {
		logger.Debug("batch_sync_start")
		if err := storedb.ApplyBatch(mainBatch, true); err != nil {
			errors = append(errors, fmt.Errorf("apply main batch: %w", err))
		} else {
			logger.Debug("batch_main_synced")
		}
		if err := storedb.ApplyIndexBatch(indexBatch, true); err != nil {
			errors = append(errors, fmt.Errorf("apply index batch: %w", err))
		} else {
			logger.Debug("batch_index_synced")
		}
		logger.Info("batch_sync_complete")
	}

	mainBatch.Close()
	indexBatch.Close()

	if len(errors) > 0 {
		for _, err := range errors {
			logger.Error("batch_flush_error", "err", err)
		}
		return fmt.Errorf("batch flush completed with %d errors", len(errors))
	}

	b.Reset()
	logger.Debug("batch_reset_complete")

	return nil
}

// Reset clears all accumulated changes after batch completion
func (b *BatchIndexManager) Reset() {
	b.threadMessages = make(map[string]*index.ThreadMessageIndexes)
	b.threadMeta = make(map[string][]byte)
	b.messageData = make(map[string]MessageData)
	b.indexData = make(map[string][]byte)
	b.messageSequencer.Reset()
}
