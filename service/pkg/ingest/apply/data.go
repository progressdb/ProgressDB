package apply

import (
	"encoding/json"
	"fmt"
	"sync"

	"progressdb/pkg/models"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/messages"
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

func (dm *DataManager) SetThreadMeta(threadID string, data interface{}) error {
	if threadID == "" {
		return fmt.Errorf("threadID cannot be empty")
	}
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}
	dm.mu.Lock()
	defer dm.mu.Unlock()
	marshaled, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal thread meta: %w", err)
	}
	dm.threadMeta[threadID] = marshaled
	return nil
}

func (dm *DataManager) DeleteThreadMeta(threadID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.threadMeta[threadID] = nil
}

func (dm *DataManager) SetMessageData(threadID, messageID string, data interface{}, ts int64, seq uint64) error {
	if threadID == "" || messageID == "" {
		return fmt.Errorf("threadID and messageID cannot be empty")
	}
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}
	dm.mu.Lock()
	defer dm.mu.Unlock()

	marshaled, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal message data: %w", err)
	}

	// Encrypt if it's a message (not for partials or other types)
	if _, ok := data.(*models.Message); ok {
		marshaled, err = encryption.EncryptMessageData(threadID, marshaled)
		if err != nil {
			return fmt.Errorf("failed to encrypt message data: %w", err)
		}
	}

	messageKey := keys.GenMessageKey(threadID, messageID, seq)
	dm.messageData[messageKey] = MessageData{
		Key:  messageKey,
		Data: marshaled,
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

func (dm *DataManager) SetVersionKey(versionKey string, data interface{}) error {
	if versionKey == "" {
		return fmt.Errorf("versionKey cannot be empty")
	}
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}
	dm.mu.Lock()
	defer dm.mu.Unlock()
	marshaled, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal version data: %w", err)
	}

	// Encrypt if it's a message
	if msg, ok := data.(*models.Message); ok {
		marshaled, err = encryption.EncryptMessageData(msg.Thread, marshaled)
		if err != nil {
			return fmt.Errorf("failed to encrypt version data: %w", err)
		}
	}

	dm.versionKeys[versionKey] = marshaled
	return nil
}

func (dm *DataManager) GetThreadMetaCopy(threadID string) ([]byte, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	data, exists := dm.threadMeta[threadID]
	if !exists {
		return nil, fmt.Errorf("thread meta not found: %s", threadID)
	}
	return append([]byte(nil), data...), nil
}

func (dm *DataManager) GetMessageDataCopy(messageKey string) ([]byte, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	msgData, exists := dm.messageData[messageKey]
	if exists {
		// Decrypt batch data
		parsed, err := keys.ParseMessageKey(messageKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse message key: %w", err)
		}
		kmsMeta, err := encryption.GetThreadKMS(parsed.ThreadID)
		if err != nil {
			return nil, fmt.Errorf("failed to get thread KMS: %w", err)
		}
		decrypted, err := encryption.DecryptMessageData(kmsMeta, msgData.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt message data: %w", err)
		}
		return append([]byte(nil), decrypted...), nil
	}

	// Not in batch, fetch from DB
	data, err := messages.GetLatestMessage(messageKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get message from DB: %w", err)
	}
	return []byte(data), nil
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
