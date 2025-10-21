package encryption

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/kms"
	"progressdb/pkg/models"
	"progressdb/pkg/security"
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

// EncryptMessageData encrypts message data if encryption is enabled.
func EncryptMessageData(thread *models.Thread, data []byte) ([]byte, error) {
	if !security.EncryptionEnabled() {
		return data, nil
	}

	if thread.KMS == nil || thread.KMS.KeyID == "" {
		return nil, fmt.Errorf("no KMS key ID for thread")
	}

	if security.EncryptionHasFieldPolicy() {
		var msg models.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		encBody, err := security.EncryptMessageBody(&msg, *thread)
		if err != nil {
			return nil, err
		}
		msg.Body = encBody
		return json.Marshal(msg)
	} else {
		enc, _, err := kms.EncryptWithDEK(thread.KMS.KeyID, data, nil)
		return enc, err
	}
}

// DecryptMessageData decrypts message data if encryption is enabled.
func DecryptMessageData(thread *models.Thread, data []byte) ([]byte, error) {
	if !security.EncryptionEnabled() {
		return data, nil
	}

	if thread.KMS == nil || thread.KMS.KeyID == "" {
		return nil, fmt.Errorf("no KMS key ID for thread")
	}

	if security.EncryptionHasFieldPolicy() {
		var msg models.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		decBody, err := security.DecryptMessageBody(&msg, thread.KMS.KeyID)
		if err != nil {
			return nil, err
		}
		msg.Body = decBody
		return json.Marshal(msg)
	} else {
		return kms.DecryptWithDEK(thread.KMS.KeyID, data, nil)
	}
}
