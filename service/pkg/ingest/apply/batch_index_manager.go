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

// BatchIndexManager manages ephemeral index updates during batch processing
// to avoid repeated read-modify-write operations and consolidate database writes.
type BatchIndexManager struct {
	mu sync.RWMutex

	// User-scoped indexes
	userThreads         map[string]*index.UserThreadIndexes
	userDeletedThreads  map[string][]string // Simplified: just track thread IDs
	userDeletedMessages map[string][]string // Simplified: just track message IDs

	// Thread-scoped indexes
	threadMessages     map[string]*index.ThreadMessageIndexes
	threadParticipants map[string]*index.ThreadParticipantIndexes

	// Message-scoped data
	messageVersions map[string][]MessageVersion

	// Direct DB operations (main DB)
	threadMeta  map[string][]byte
	messageData map[string]MessageData

	// Index DB operations
	indexData map[string][]byte
}

// MessageVersion represents a version entry for batching
type MessageVersion struct {
	Key  string
	Data []byte
	TS   int64
	Seq  uint64
}

// MessageData represents message data for batching
type MessageData struct {
	Key  string
	Data []byte
	TS   int64
	Seq  uint64
}

// NewBatchIndexManager creates a new batch index manager
func NewBatchIndexManager() *BatchIndexManager {
	return &BatchIndexManager{
		userThreads:         make(map[string]*index.UserThreadIndexes),
		userDeletedThreads:  make(map[string][]string),
		userDeletedMessages: make(map[string][]string),
		threadMessages:      make(map[string]*index.ThreadMessageIndexes),
		threadParticipants:  make(map[string]*index.ThreadParticipantIndexes),
		messageVersions:     make(map[string][]MessageVersion),
		threadMeta:          make(map[string][]byte),
		messageData:         make(map[string]MessageData),
		indexData:           make(map[string][]byte),
	}
}

// AddThreadToUser adds a thread to user's ownership index
func (b *BatchIndexManager) AddThreadToUser(userID, threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.userThreads[userID] == nil {
		b.userThreads[userID] = &index.UserThreadIndexes{Threads: []string{}}
	}

	// Add if not present
	for _, t := range b.userThreads[userID].Threads {
		if t == threadID {
			return // already added
		}
	}
	b.userThreads[userID].Threads = append(b.userThreads[userID].Threads, threadID)
}

// RemoveThreadFromUser removes a thread from user's ownership index
func (b *BatchIndexManager) RemoveThreadFromUser(userID, threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.userThreads[userID] == nil {
		return
	}

	threads := b.userThreads[userID].Threads
	for i, t := range threads {
		if t == threadID {
			b.userThreads[userID].Threads = append(threads[:i], threads[i+1:]...)
			break
		}
	}
}

// AddDeletedThreadToUser adds a thread to user's deleted threads
func (b *BatchIndexManager) AddDeletedThreadToUser(userID, threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.userDeletedThreads[userID] == nil {
		b.userDeletedThreads[userID] = []string{}
	}
	b.userDeletedThreads[userID] = append(b.userDeletedThreads[userID], threadID)
}

// AddDeletedMessageToUser adds a message to user's deleted messages
func (b *BatchIndexManager) AddDeletedMessageToUser(userID, msgID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.userDeletedMessages[userID] == nil {
		b.userDeletedMessages[userID] = []string{}
	}
	b.userDeletedMessages[userID] = append(b.userDeletedMessages[userID], msgID)
}

// InitThreadMessageIndexes initializes message indexes for a new thread
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

// UpdateThreadMessageIndexes updates thread message indexes for save/delete operations
func (b *BatchIndexManager) UpdateThreadMessageIndexes(threadID string, createdAt, updatedAt int64, isDelete bool, msgKey string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	idx := b.threadMessages[threadID]
	if idx == nil {
		// Initialize if not exists
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
		// Add to skips
		idx.Skips = append(idx.Skips, msgKey)
	} else {
		// Update counts and timestamps
		if idx.LastCreatedAt == 0 || createdAt < idx.LastCreatedAt {
			idx.LastCreatedAt = createdAt
		}
		if updatedAt > idx.LastUpdatedAt {
			idx.LastUpdatedAt = updatedAt
		}
		idx.End++
		// Add deltas (simplified - would need more sophisticated logic)
		idx.Cdeltas = append(idx.Cdeltas, 1)
		idx.Udeltas = append(idx.Udeltas, 1)
	}
}

// AddParticipantToThread adds a user to thread participants
func (b *BatchIndexManager) AddParticipantToThread(threadID, userID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.threadParticipants[threadID] == nil {
		b.threadParticipants[threadID] = &index.ThreadParticipantIndexes{Participants: []string{}}
	}

	// Add if not present
	for _, p := range b.threadParticipants[threadID].Participants {
		if p == userID {
			return // already added
		}
	}
	b.threadParticipants[threadID].Participants = append(b.threadParticipants[threadID].Participants, userID)
}

// SetThreadMeta sets thread metadata directly
func (b *BatchIndexManager) SetThreadMeta(threadID string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Store by thread ID, the key will be generated during flush
	b.threadMeta[threadID] = append([]byte(nil), data...)
}

// SetMessageData sets message data directly
func (b *BatchIndexManager) SetMessageData(threadID string, msgID string, data []byte, ts int64, seq uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key, err := keys.MsgKey(threadID, ts, seq)
	if err != nil {
		return fmt.Errorf("message key generation failed: %w", err)
	}

	b.messageData[key] = MessageData{
		Key:  key,
		Data: append([]byte(nil), data...),
		TS:   ts,
		Seq:  seq,
	}
	return nil
}

// AddMessageVersion adds a message version for batching
func (b *BatchIndexManager) AddMessageVersion(msgID string, data []byte, ts int64, seq uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key, err := keys.VersionKey(msgID, ts, seq)
	if err != nil {
		return fmt.Errorf("version key generation failed: %w", err)
	}

	if b.messageVersions[msgID] == nil {
		b.messageVersions[msgID] = []MessageVersion{}
	}

	b.messageVersions[msgID] = append(b.messageVersions[msgID], MessageVersion{
		Key:  key,
		Data: append([]byte(nil), data...),
		TS:   ts,
		Seq:  seq,
	})
	return nil
}

// SetIndexData sets arbitrary index data for batching
func (b *BatchIndexManager) SetIndexData(key string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.indexData[key] = append([]byte(nil), data...)
}

// DeleteThreadMeta marks thread metadata for deletion
func (b *BatchIndexManager) DeleteThreadMeta(threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Store by thread ID, the key will be generated during flush
	b.threadMeta[threadID] = nil // Mark for deletion
}

// DeleteThreadMessageIndexes marks thread message indexes for deletion
func (b *BatchIndexManager) DeleteThreadMessageIndexes(threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.threadMessages[threadID] = nil // Mark for deletion
}

// Flush persists all accumulated changes to the database atomically
func (b *BatchIndexManager) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	logger.Debug("batch_flush_accumulated",
		"threads", len(b.threadMeta),
		"messages", len(b.messageData),
		"versions", len(b.messageVersions),
		"user_threads", len(b.userThreads))

	var errors []error

	// Create batch for atomic operations
	mainBatch := storedb.Client.NewBatch()
	indexBatch := index.IndexDB.NewBatch()

	// Flush thread metadata
	for threadID, data := range b.threadMeta {
		threadKey, err := keys.ThreadMetaKey(threadID)
		if err != nil {
			errors = append(errors, fmt.Errorf("thread meta key %s: %w", threadID, err))
			continue
		}

		if data == nil {
			// Delete operation
			if err := mainBatch.Delete([]byte(threadKey), storedb.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("delete thread meta %s: %w", threadID, err))
			}
		} else {
			// Set operation
			if err := mainBatch.Set([]byte(threadKey), data, storedb.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set thread meta %s: %w", threadID, err))
			}
		}
	}

	// Flush message data
	for key, msgData := range b.messageData {
		if err := mainBatch.Set([]byte(key), msgData.Data, storedb.WriteOpt(true)); err != nil {
			errors = append(errors, fmt.Errorf("set message data %s: %w", key, err))
		}
	}

	// Flush message versions
	for _, versions := range b.messageVersions {
		for _, version := range versions {
			if err := mainBatch.Set([]byte(version.Key), version.Data, storedb.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set message version %s: %w", version.Key, err))
			}
		}
	}

	// Flush user thread indexes
	for userID, userIdx := range b.userThreads {
		if userIdx != nil {
			data, err := json.Marshal(userIdx)
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal user threads %s: %w", userID, err))
				continue
			}
			userKey, err := keys.UserThreadsIndexKey(userID)
			if err != nil {
				errors = append(errors, fmt.Errorf("user threads key %s: %w", userID, err))
				continue
			}
			if err := indexBatch.Set([]byte(userKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set user threads %s: %w", userID, err))
			}
		}
	}

	// Flush deleted threads
	for userID, deletedThreads := range b.userDeletedThreads {
		if len(deletedThreads) > 0 {
			data, err := json.Marshal(deletedThreads)
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal deleted threads %s: %w", userID, err))
				continue
			}
			deletedKey, err := keys.DeletedThreadsIndexKey(userID)
			if err != nil {
				errors = append(errors, fmt.Errorf("deleted threads key %s: %w", userID, err))
				continue
			}
			if err := indexBatch.Set([]byte(deletedKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set deleted threads %s: %w", userID, err))
			}
		}
	}

	// Flush deleted messages
	for userID, deletedMessages := range b.userDeletedMessages {
		if len(deletedMessages) > 0 {
			data, err := json.Marshal(deletedMessages)
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal deleted messages %s: %w", userID, err))
				continue
			}
			deletedKey, err := keys.DeletedMessagesIndexKey(userID)
			if err != nil {
				errors = append(errors, fmt.Errorf("deleted messages key %s: %w", userID, err))
				continue
			}
			if err := indexBatch.Set([]byte(deletedKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set deleted messages %s: %w", userID, err))
			}
		}
	}

	// Flush thread message indexes
	for threadID, threadIdx := range b.threadMessages {
		threadKey, err := keys.ThreadsToMessagesIndexKey(threadID, "")
		if err != nil {
			errors = append(errors, fmt.Errorf("thread messages key %s: %w", threadID, err))
			continue
		}

		if threadIdx == nil {
			// Delete operation
			if err := indexBatch.Delete([]byte(threadKey), index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("delete thread messages %s: %w", threadID, err))
			}
		} else {
			// Set operation
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

	// Flush thread participants
	for threadID, participantIdx := range b.threadParticipants {
		if participantIdx != nil {
			data, err := json.Marshal(participantIdx)
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal thread participants %s: %w", threadID, err))
				continue
			}
			participantKey, err := keys.ThreadParticipantsIndexKey(threadID)
			if err != nil {
				errors = append(errors, fmt.Errorf("thread participants key %s: %w", threadID, err))
				continue
			}
			if err := indexBatch.Set([]byte(participantKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set thread participants %s: %w", threadID, err))
			}
		}
	}

	// Flush arbitrary index data
	for key, data := range b.indexData {
		if err := indexBatch.Set([]byte(key), data, index.WriteOpt(true)); err != nil {
			errors = append(errors, fmt.Errorf("set index data %s: %w", key, err))
		}
	}

	// Apply batches atomically
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

	// Close batches
	mainBatch.Close()
	indexBatch.Close()

	// Log any errors that occurred
	if len(errors) > 0 {
		for _, err := range errors {
			logger.Error("batch_flush_error", "err", err)
		}
		return fmt.Errorf("batch flush completed with %d errors", len(errors))
	}

	// Reset after successful flush
	b.Reset()
	logger.Debug("batch_reset_complete")

	return nil
}

// Reset clears all accumulated changes after batch completion
func (b *BatchIndexManager) Reset() {
	b.userThreads = make(map[string]*index.UserThreadIndexes)
	b.userDeletedThreads = make(map[string][]string)
	b.userDeletedMessages = make(map[string][]string)
	b.threadMessages = make(map[string]*index.ThreadMessageIndexes)
	b.threadParticipants = make(map[string]*index.ThreadParticipantIndexes)
	b.messageVersions = make(map[string][]MessageVersion)
	b.threadMeta = make(map[string][]byte)
	b.messageData = make(map[string]MessageData)
	b.indexData = make(map[string][]byte)
}
