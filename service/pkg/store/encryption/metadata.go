package encryption

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"
)

func ProvisionThreadKMS(threadKey string) (*models.KMSMeta, error) {
	if !EncryptionEnabled() {
		return nil, nil
	}

	if !IsProviderEnabled() {
		logger.Info("encryption_enabled_but_no_kms_provider", "thread", threadKey)
		return nil, nil
	}

	logger.Info("provisioning_thread_kms", "thread", threadKey)
	keyID, wrapped, kekID, kekVer, err := CreateDEK(threadKey)
	if err != nil {
		return nil, fmt.Errorf("kms provision failed: %w", err)
	}

	// Convert wrapped DEK to base64 string for storage
	wrappedStr := base64.StdEncoding.EncodeToString(wrapped)

	return &models.KMSMeta{
		KeyID:      keyID,
		WrappedDEK: wrappedStr,
		KEKID:      kekID,
		KEKVersion: kekVer,
	}, nil
}

func GetThreadKMS(threadKey string) (*models.KMSMeta, error) {
	if !EncryptionEnabled() {
		return nil, nil
	}

	threadKey = keys.GenThreadKey(threadKey)

	threadData, closer, err := storedb.Client.Get([]byte(threadKey))
	if err != nil {
		if storedb.IsNotFound(err) {
			return nil, fmt.Errorf("thread not found: %s", threadKey)
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
