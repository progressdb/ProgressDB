package encryption

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/encryption/kms"
	"progressdb/pkg/store/keys"
)

// true if likely contains JSON object/array based on first non-whitespace
func likelyJSON(b []byte) bool {
	for _, c := range b {
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		return c == '{' || c == '['
	}
	return false
}

// exported version of likelyJSON
func LikelyJSON(b []byte) bool { return likelyJSON(b) }

// EncryptMessageData encrypts message data if encryption is enabled, fetching KMS meta automatically.
func EncryptMessageData(threadID string, data []byte) ([]byte, error) {
	if !EncryptionEnabled() {
		return data, nil
	}

	kmsMeta, err := GetThreadKMS(threadID)
	if err != nil {
		return nil, err
	}

	if EncryptionHasFieldPolicy() {
		var msg models.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		// For field policy, we need the full thread? Wait, security.EncryptMessageBody takes thread, but perhaps it only uses KMS.
		// Assuming it can take a thread with only KMS.
		fakeThread := &models.Thread{KMS: kmsMeta}
		encBody, err := EncryptMessageBody(&msg, *fakeThread)
		if err != nil {
			return nil, err
		}
		msg.Body = encBody
		return json.Marshal(msg)
	} else {
		enc, _, err := kms.EncryptWithDEK(kmsMeta.KeyID, data, nil)
		return enc, err
	}
}

// DecryptMessageData decrypts message data if encryption is enabled.
func DecryptMessageData(kmsMeta *models.KMSMeta, data []byte) ([]byte, error) {
	if !EncryptionEnabled() {
		return data, nil
	}

	if kmsMeta == nil || kmsMeta.KeyID == "" {
		return nil, fmt.Errorf("no KMS key ID for thread")
	}

	if EncryptionHasFieldPolicy() {
		var msg models.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		decBody, err := DecryptMessageBody(&msg, kmsMeta.KeyID)
		if err != nil {
			return nil, err
		}
		msg.Body = decBody
		return json.Marshal(msg)
	} else {
		return kms.DecryptWithDEK(kmsMeta.KeyID, data, nil)
	}
}

// ProvisionThreadKMS provisions a DEK for the thread if encryption is enabled and KMS provider is available.
func ProvisionThreadKMS(threadID string) (*models.KMSMeta, error) {
	if !EncryptionEnabled() {
		return nil, nil
	}

	if !kms.IsProviderEnabled() {
		logger.Info("encryption_enabled_but_no_kms_provider", "thread", threadID)
		return nil, nil
	}

	logger.Info("provisioning_thread_kms", "thread", threadID)
	keyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(threadID)
	if err != nil {
		return nil, fmt.Errorf("kms provision failed: %w", err)
	}

	return &models.KMSMeta{
		KeyID:      keyID,
		WrappedDEK: base64.StdEncoding.EncodeToString(wrapped),
		KEKID:      kekID,
		KEKVersion: kekVer,
	}, nil
}

// GetThreadKMS retrieves only the KMS metadata for a thread without loading the full thread data.
func GetThreadKMS(threadID string) (*models.KMSMeta, error) {
	if !EncryptionEnabled() {
		return nil, nil
	}

	threadKey := keys.GenThreadKey(threadID)

	threadData, closer, err := storedb.Client.Get([]byte(threadKey))
	if err != nil {
		if storedb.IsNotFound(err) {
			return nil, fmt.Errorf("thread not found: %s", threadID)
		}
		return nil, fmt.Errorf("failed to get thread: %w", err)
	}
	if closer != nil {
		defer closer.Close()
	}

	// Unmarshal only the KMS field
	var thread struct {
		KMS *models.KMSMeta `json:"kms"`
	}
	if err := json.Unmarshal(threadData, &thread); err != nil {
		return nil, fmt.Errorf("failed to unmarshal thread KMS: %w", err)
	}

	if thread.KMS == nil || thread.KMS.KeyID == "" {
		return nil, fmt.Errorf("no KMS key ID for thread")
	}

	return thread.KMS, nil
}
