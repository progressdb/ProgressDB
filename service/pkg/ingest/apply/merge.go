package apply

import (
	"progressdb/pkg/store/db/index"
	"progressdb/pkg/store/keys"
)

// Data merging operations - temporary hold of apply data into their final threadID to value mappings
// These methods handle the accumulation of data changes before batch persistence
//
// This file contains all data accumulation methods:
// - Message data storage (SetMessageData)
// - Version management (AddMessageVersion)
// - User ownership tracking (AddThreadToUser, RemoveThreadFromUser)
// - Participant management (AddParticipantToThread)
// - Deletion tracking (AddDeletedThreadToUser, AddDeletedMessageToUser)
// - Thread metadata (SetThreadMeta, DeleteThreadMeta)

// Generic index data
func (b *BatchIndexManager) SetIndexData(key string, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.indexData[key] = append([]byte(nil), data...)
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
