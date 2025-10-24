package apply

import (
	"progressdb/pkg/store/db/index"
	"progressdb/pkg/store/keys"
)

// Data merging operations - temporary hold of apply data into their final threadID to value mappings
// These methods handle the accumulation of data changes before batch persistence

// User-thread relationship management
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

// Deleted item tracking
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

// Thread participant management
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

// Thread metadata operations
func (b *BatchIndexManager) SetThreadMeta(threadID string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.threadMeta[threadID] = append([]byte(nil), data...)
}

func (b *BatchIndexManager) DeleteThreadMeta(threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.threadMeta[threadID] = nil
}

// Message data storage
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

// Version management
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

// Generic index data
func (b *BatchIndexManager) SetIndexData(key string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.indexData[key] = append([]byte(nil), data...)
}

// Index cleanup
func (b *BatchIndexManager) DeleteThreadMessageIndexes(threadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.threadMessages[threadID] = nil
}
