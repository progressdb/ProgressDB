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
	mu                  sync.RWMutex
	userThreads         map[string]*index.UserThreadIndexes
	userDeletedThreads  map[string][]string
	userDeletedMessages map[string][]string
	userThreadSequences map[string]uint64
	threadMessages      map[string]*index.ThreadMessageIndexes
	threadParticipants  map[string]*index.ThreadParticipantIndexes
	messageVersions     map[string][]MessageVersion
	threadMeta          map[string][]byte
	messageData         map[string]MessageData
	indexData           map[string][]byte
	messageSequencer    *MessageSequencer
}
type MessageVersion struct {
	Key  string
	Data []byte
	TS   int64
	Seq  uint64
}
type MessageData struct {
	Key  string
	Data []byte
	TS   int64
	Seq  uint64
}

func NewBatchIndexManager() *BatchIndexManager {
	return &BatchIndexManager{
		userThreads:         make(map[string]*index.UserThreadIndexes),
		userDeletedThreads:  make(map[string][]string),
		userDeletedMessages: make(map[string][]string),
		userThreadSequences: make(map[string]uint64),
		threadMessages:      make(map[string]*index.ThreadMessageIndexes),
		threadParticipants:  make(map[string]*index.ThreadParticipantIndexes),
		messageVersions:     make(map[string][]MessageVersion),
		threadMeta:          make(map[string][]byte),
		messageData:         make(map[string]MessageData),
		indexData:           make(map[string][]byte),
		messageSequencer:    NewMessageSequencer(),
	}
}

func (b *BatchIndexManager) GetNextUserThreadSequence(userID string) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.userThreadSequences[userID]; !exists {
		b.userThreadSequences[userID] = 0
	}

	b.userThreadSequences[userID]++
	return b.userThreadSequences[userID]
}

func (b *BatchIndexManager) SetUserThreadSequence(userID string, sequence uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.userThreadSequences[userID] = sequence
}

func (b *BatchIndexManager) InitializeUserSequencesFromDB(userIDs []string) error {
	for _, userID := range userIDs {
		threads, err := index.GetUserThreads(userID)
		if err != nil {
			return fmt.Errorf("get user threads %s: %w", userID, err)
		}
		b.SetUserThreadSequence(userID, uint64(len(threads)))
	}
	return nil
}

func (b *BatchIndexManager) MapProvisionalToFinalID(provisionalID, finalID string) {
	logger.Debug("mapped_provisional_thread", "provisional", provisionalID, "final", finalID)
}

func (b *BatchIndexManager) GetFinalThreadKey(threadKey string) (string, error) {
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return "", fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", threadKey)
	}
	return threadKey, nil
}

func (b *BatchIndexManager) GetFinalMessageKey(messageKey string) (string, error) {
	return b.messageSequencer.GetFinalMessageKey(messageKey)
}

func (b *BatchIndexManager) MapProvisionalToFinalMessageKey(provisionalKey, finalKey string) {
	b.messageSequencer.MapProvisionalToFinalMessageKey(provisionalKey, finalKey)
}

func (b *BatchIndexManager) AddThreadToUser(userID, threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.userThreads[userID] == nil {
		b.userThreads[userID] = &index.UserThreadIndexes{Threads: []string{}}
	}

	for _, t := range b.userThreads[userID].Threads {
		if t == threadID {
			return
		}
	}
	b.userThreads[userID].Threads = append(b.userThreads[userID].Threads, threadID)
}

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

func (b *BatchIndexManager) AddDeletedThreadToUser(userID, threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.userDeletedThreads[userID] == nil {
		b.userDeletedThreads[userID] = []string{}
	}
	b.userDeletedThreads[userID] = append(b.userDeletedThreads[userID], threadID)
}

func (b *BatchIndexManager) AddDeletedMessageToUser(userID, msgID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.userDeletedMessages[userID] == nil {
		b.userDeletedMessages[userID] = []string{}
	}
	b.userDeletedMessages[userID] = append(b.userDeletedMessages[userID], msgID)
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

func (b *BatchIndexManager) AddParticipantToThread(threadID, userID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.threadParticipants[threadID] == nil {
		b.threadParticipants[threadID] = &index.ThreadParticipantIndexes{Participants: []string{}}
	}

	for _, p := range b.threadParticipants[threadID].Participants {
		if p == userID {
			return
		}
	}
	b.threadParticipants[threadID].Participants = append(b.threadParticipants[threadID].Participants, userID)
}

func (b *BatchIndexManager) SetThreadMeta(threadID string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.threadMeta[threadID] = append([]byte(nil), data...)
}

func (b *BatchIndexManager) SetMessageData(threadID string, msgID string, data []byte, ts int64, seq uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := keys.GenMessageKey(threadID, msgID, seq)

	b.messageData[key] = MessageData{
		Key:  key,
		Data: append([]byte(nil), data...),
		TS:   ts,
		Seq:  seq,
	}
	return nil
}

func (b *BatchIndexManager) AddMessageVersion(msgID string, data []byte, ts int64, seq uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := keys.GenVersionKey(msgID, ts, seq)

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

func (b *BatchIndexManager) SetIndexData(key string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.indexData[key] = append([]byte(nil), data...)
}

func (b *BatchIndexManager) DeleteThreadMeta(threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.threadMeta[threadID] = nil
}

func (b *BatchIndexManager) DeleteThreadMessageIndexes(threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.threadMessages[threadID] = nil
}

func (b *BatchIndexManager) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	logger.Debug("batch_flush_accumulated",
		"threads", len(b.threadMeta),
		"messages", len(b.messageData),
		"versions", len(b.messageVersions),
		"user_threads", len(b.userThreads))

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

	for _, versions := range b.messageVersions {
		for _, version := range versions {
			if err := mainBatch.Set([]byte(version.Key), version.Data, storedb.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set message version %s: %w", version.Key, err))
			}
		}
	}

	for userID, userIdx := range b.userThreads {
		if userIdx != nil {
			data, err := json.Marshal(userIdx)
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal user threads %s: %w", userID, err))
				continue
			}
			userKey := keys.GenUserThreadsKey(userID)
			if err := indexBatch.Set([]byte(userKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set user threads %s: %w", userID, err))
			}
		}
	}

	for userID, deletedThreads := range b.userDeletedThreads {
		if len(deletedThreads) > 0 {
			data, err := json.Marshal(deletedThreads)
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal deleted threads %s: %w", userID, err))
				continue
			}
			deletedKey := keys.GenDeletedThreadsKey(userID)
			if err := indexBatch.Set([]byte(deletedKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set deleted threads %s: %w", userID, err))
			}
		}
	}

	for userID, deletedMessages := range b.userDeletedMessages {
		if len(deletedMessages) > 0 {
			data, err := json.Marshal(deletedMessages)
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal deleted messages %s: %w", userID, err))
				continue
			}
			deletedKey := keys.GenDeletedMessagesKey(userID)
			if err := indexBatch.Set([]byte(deletedKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set deleted messages %s: %w", userID, err))
			}
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

	for threadID, participantIdx := range b.threadParticipants {
		if participantIdx != nil {
			data, err := json.Marshal(participantIdx)
			if err != nil {
				errors = append(errors, fmt.Errorf("marshal thread participants %s: %w", threadID, err))
				continue
			}
			participantKey := keys.GenThreadParticipantsKey(threadID)
			if err := indexBatch.Set([]byte(participantKey), data, index.WriteOpt(true)); err != nil {
				errors = append(errors, fmt.Errorf("set thread participants %s: %w", threadID, err))
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
	b.userThreads = make(map[string]*index.UserThreadIndexes)
	b.userDeletedThreads = make(map[string][]string)
	b.userDeletedMessages = make(map[string][]string)
	b.userThreadSequences = make(map[string]uint64)
	b.threadMessages = make(map[string]*index.ThreadMessageIndexes)
	b.threadParticipants = make(map[string]*index.ThreadParticipantIndexes)
	b.messageVersions = make(map[string][]MessageVersion)
	b.threadMeta = make(map[string][]byte)
	b.messageData = make(map[string]MessageData)
	b.indexData = make(map[string][]byte)
	b.messageSequencer.Reset()
}
