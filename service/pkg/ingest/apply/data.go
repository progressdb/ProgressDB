package apply

import (
	"sync"

	"progressdb/pkg/store/keys"
)

type MessageData struct {
	Key  string
	Data []byte
	TS   int64
	Seq  uint64
}

type DataManager struct {
	mu          sync.RWMutex
	threadMeta  map[string][]byte
	messageData map[string]MessageData
	versionKeys map[string][]byte // versionKey -> versionData
}

func NewDataManager() *DataManager {
	return &DataManager{
		threadMeta:  make(map[string][]byte),
		messageData: make(map[string]MessageData),
		versionKeys: make(map[string][]byte),
	}
}

func (dm *DataManager) SetThreadMeta(threadID string, data []byte) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.threadMeta[threadID] = data
}

func (dm *DataManager) DeleteThreadMeta(threadID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.threadMeta[threadID] = nil
}

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

func (dm *DataManager) GetThreadMeta() map[string][]byte {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string][]byte)
	for k, v := range dm.threadMeta {
		result[k] = append([]byte(nil), v...)
	}
	return result
}

func (dm *DataManager) GetMessageData() map[string]MessageData {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string]MessageData)
	for k, v := range dm.messageData {
		result[k] = v
	}
	return result
}

func (dm *DataManager) SetVersionKey(versionKey string, data []byte) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.versionKeys[versionKey] = data
}

func (dm *DataManager) GetVersionKeys() map[string][]byte {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string][]byte)
	for k, v := range dm.versionKeys {
		result[k] = append([]byte(nil), v...)
	}
	return result
}

func (dm *DataManager) Reset() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.threadMeta = make(map[string][]byte)
	dm.messageData = make(map[string]MessageData)
	dm.versionKeys = make(map[string][]byte)
}
