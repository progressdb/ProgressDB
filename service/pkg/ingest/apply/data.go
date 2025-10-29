package apply

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/models"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/features/messages"
	"progressdb/pkg/store/features/threads"
	"progressdb/pkg/store/keys"
)

type DataManager struct {
	kv *KVManager
}

func NewDataManager(kv *KVManager) *DataManager {
	return &DataManager{
		kv: kv,
	}
}

func (dm *DataManager) SetThreadData(threadKey string, data interface{}) error {
	if threadKey == "" {
		return fmt.Errorf("threadKey cannot be empty")
	}
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}
	marshaled, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal thread meta: %w", err)
	}
	dm.kv.SetStoreKV(keys.GenThreadKey(threadKey), marshaled)
	return nil
}

func (dm *DataManager) SetMessageData(messageKey string, data interface{}, ts int64) error {
	if messageKey == "" {
		return fmt.Errorf("messageKey cannot be empty")
	}
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}

	// Parse the message key to extract components
	parts, err := keys.ParseMessageKey(messageKey)
	if err != nil {
		return fmt.Errorf("parse message key: %w", err)
	}

	marshaled, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal message data: %w", err)
	}

	// Encrypt if it's a message (not for partials or other types)
	if _, ok := data.(*models.Message); ok {
		marshaled, err = encryption.EncryptMessageData(parts.ThreadID, marshaled)
		if err != nil {
			return fmt.Errorf("failed to encrypt message data: %w", err)
		}
	}

	dm.kv.SetStoreKV(messageKey, marshaled)
	return nil
}

func (dm *DataManager) SetVersionKey(versionKey string, data interface{}) error {
	if versionKey == "" {
		return fmt.Errorf("versionKey cannot be empty")
	}
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}
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

	dm.kv.SetIndexKV(versionKey, marshaled)
	return nil
}

func (dm *DataManager) GetThreadMetaCopy(threadKey string) ([]byte, error) {
	if data, ok := dm.kv.GetStoreKV(keys.GenThreadKey(threadKey)); ok && data != nil {
		return append([]byte(nil), data...), nil
	}

	// Not in batch or deleted, fetch from DB
	dataStr, err := threads.GetThread(threadKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread from DB: %w", err)
	}
	return []byte(dataStr), nil
}

func (dm *DataManager) GetMessageDataCopy(messageKey string) ([]byte, error) {
	// NOTE: no decryption, as .body is not used but replaced & stored.

	if data, ok := dm.kv.GetStoreKV(messageKey); ok {
		return append([]byte(nil), data...), nil
	}

	// Not in batch, fetch from DB
	data, err := messages.GetLatestMessage(messageKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get message from DB: %w", err)
	}
	return []byte(data), nil
}
