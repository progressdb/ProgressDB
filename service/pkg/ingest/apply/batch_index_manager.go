package apply

import (
	"sync"

	"progressdb/pkg/store/db/index"
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

	b.threadMeta[threadID] = append([]byte(nil), data...)
}

// SetMessageData sets message data directly
func (b *BatchIndexManager) SetMessageData(threadID string, msgID string, data []byte, ts int64, seq uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	key, err := keys.MsgKey(threadID, ts, seq)
	if err != nil {
		return // TODO: handle error properly
	}

	b.messageData[key] = MessageData{
		Key:  key,
		Data: append([]byte(nil), data...),
		TS:   ts,
		Seq:  seq,
	}
}

// AddMessageVersion adds a message version for batching
func (b *BatchIndexManager) AddMessageVersion(msgID string, data []byte, ts int64, seq uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	key, err := keys.VersionKey(msgID, ts, seq)
	if err != nil {
		return // TODO: handle error properly
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
}

// SetIndexData sets arbitrary index data for batching
func (b *BatchIndexManager) SetIndexData(key string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.indexData[key] = append([]byte(nil), data...)
}

// Reset clears all accumulated changes after batch completion
func (b *BatchIndexManager) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

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
