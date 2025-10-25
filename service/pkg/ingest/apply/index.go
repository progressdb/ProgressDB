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

// Index management and batch operations
// This file contains:
// - Thread message index management (UpdateThreadMessageIndexes, GetNextThreadSequence)
// - Thread sequence initialization (InitializeThreadSequencesFromDB)
// - Batch flushing logic (Flush)
// - Index data structures and initialization

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
	bim := &BatchIndexManager{
		threadMessages: make(map[string]*index.ThreadMessageIndexes),
		threadMeta:     make(map[string][]byte),
		messageData:    make(map[string]MessageData),
		indexData:      make(map[string][]byte),
	}
	bim.messageSequencer = NewMessageSequencer(bim)
	return bim
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
func (b *BatchIndexManager) GetNextThreadSequence(threadID string) uint64 {
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

	idx.End++
	return idx.End
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
