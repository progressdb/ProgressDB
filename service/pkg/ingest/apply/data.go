package apply

import (
	"sync"

	"progressdb/pkg/store/keys"
)

// MessageData represents stored message information
type MessageData struct {
	Key  string
	Data []byte
	TS   int64
	Seq  uint64
}

// DataManager handles accumulation of full body data (thread metadata, message data)
// This file contains all data accumulation methods:
// - Thread metadata (SetThreadMeta, DeleteThreadMeta)
// - Message data storage (SetMessageData)
// - Index data storage (SetIndexData)
// - Thread message index management (InitThreadMessageIndexes, DeleteThreadMessageIndexes)
//
// Note: Direct index database writes have been moved to bmuts.go as direct index calls
// since they are not ephemeral state management

type DataManager struct {
	mu          sync.RWMutex
	threadMeta  map[string][]byte
	messageData map[string]MessageData
}

// NewDataManager creates a new data manager with initialized maps
func NewDataManager() *DataManager {
	return &DataManager{
		threadMeta:  make(map[string][]byte),
		messageData: make(map[string]MessageData),
	}
}

// SetIndexData sets generic index data
func (dm *DataManager) SetIndexData(key string, data []byte) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Note: This method is kept for compatibility but index data should be handled by IndexManager
	// This will be removed in future refactoring
}

// SetThreadMeta sets thread metadata
func (dm *DataManager) SetThreadMeta(threadID string, data []byte) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.threadMeta[threadID] = data
}

// DeleteThreadMeta deletes thread metadata
func (dm *DataManager) DeleteThreadMeta(threadID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.threadMeta[threadID] = nil
}

// DeleteThreadMessageIndexes deletes thread message indexes
func (dm *DataManager) DeleteThreadMessageIndexes(threadID string) {
	// Note: This method should be handled by IndexManager, not DataManager
	// Keeping for compatibility during migration
}

// SetMessageData sets message data
func (dm *DataManager) SetMessageData(threadID, messageID string, data []byte, ts int64, seq uint64) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	messageKey := keys.GenMessageKey(threadID, messageID, seq)
	dm.messageData[messageKey] = MessageData{
		Key:  messageKey,
		Data: data,
		TS:   ts,
		Seq:  seq,
	}
	return nil
}

// GetThreadMeta returns current thread metadata state
func (dm *DataManager) GetThreadMeta() map[string][]byte {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string][]byte)
	for k, v := range dm.threadMeta {
		result[k] = append([]byte(nil), v...)
	}
	return result
}

// GetMessageData returns current message data state
func (dm *DataManager) GetMessageData() map[string]MessageData {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string]MessageData)
	for k, v := range dm.messageData {
		result[k] = v
	}
	return result
}

// Reset clears all accumulated data changes
func (dm *DataManager) Reset() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.threadMeta = make(map[string][]byte)
	dm.messageData = make(map[string]MessageData)
}
