package security

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/telemetry"
)

var key []byte
var keyLocked bool

// fieldRule represents a path to encrypt, split into segments.
type fieldRule struct {
	segments []string
}

var fieldRules []fieldRule

// SetKeyEncryptionHex sets the AES-256 master key (hex string). An empty string clears it.
func SetKeyEncryptionHex(hexKey string) error {
	// If the hexKey is empty, clear the key and unlock memory if needed.
	if hexKey == "" {
		if key != nil && keyLocked {
			_ = UnlockMemory(key)
			keyLocked = false
		}
		key = nil
		return nil
	}
	// Decode the hex string to bytes.
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return err
	}
	// Check that the key is 32 bytes (AES-256).
	if l := len(b); l != 32 {
		return errors.New("encryption key must be 32 bytes (AES-256)")
	}
	// Unlock previous key if set and locked.
	if key != nil && keyLocked {
		_ = UnlockMemory(key)
		keyLocked = false
	}
	// Set the new key and lock it in memory if possible.
	key = b
	if err := LockMemory(key); err == nil {
		keyLocked = true
	}
	return nil
}

// EncryptionEnabled returns true if encryption is enabled (KMS or local key).
func EncryptionEnabled() bool {
	// If a KMS provider is enabled, encryption is enabled.
	if kms.IsProviderEnabled() {
		return true
	}
	// Otherwise, check if a 32-byte key is set.
	return len(key) == 32
}

// SetEncryptionFieldPolicy sets the list of field paths to encrypt.
// All segments must start with "body" (e.g. "body.value", "body.content.foo").
func SetEncryptionFieldPolicy(fields []string) error {
	// Clear any existing field rules.
	fieldRules = fieldRules[:0]
	for _, p := range fields {
		// Remove leading/trailing whitespace.
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Split the path into segments by dot.
		segments := strings.Split(p, ".")
		// Always require "body" as the first segment for consistency.
		// Example: "body.value" is valid, "value" is not.
		if len(segments) == 0 || segments[0] != "body" {
			return fmt.Errorf("encryption field path must start with 'body': %q", p)
		}
		// Add the rule to the list.
		fieldRules = append(fieldRules, fieldRule{segments: segments})
	}
	return nil
}

// EncryptionHasFieldPolicy returns true if any field rules are set.
func EncryptionHasFieldPolicy() bool {
	// Returns true if there is at least one field rule.
	return len(fieldRules) > 0
}

// encryptBodyPath recursively encrypts the value at the given path.
func encryptBodyPath(bodyNode interface{}, segments []string, keyID string) interface{} {
	// If there are no more segments, encrypt the value.
	if len(segments) == 0 {
		// Marshal the value to JSON.
		raw, err := json.Marshal(bodyNode)
		if err != nil {
			return bodyNode
		}
		// Encrypt using the KMS DEK.
		ct, _, err := kms.EncryptWithDEK(keyID, raw, nil)
		if err != nil {
			return bodyNode
		}
		// Return the encrypted object.
		// Example: {"_enc": "gcm", "v": "<base64-ct>"}
		return map[string]interface{}{"_enc": "gcm", "v": base64.StdEncoding.EncodeToString(ct)}
	}

	// If there are more segments, traverse the structure.
	switch cur := bodyNode.(type) {
	case map[string]interface{}:
		segment := segments[0]
		// If the segment is "*", apply to all keys at this level.
		// Example: path ["body", "*", "foo"] will encrypt all body.<any>.foo fields.
		if segment == "*" {
			for k, child := range cur {
				cur[k] = encryptBodyPath(child, segments[1:], keyID)
			}
			return cur
		}
		// Otherwise, descend into the named field if it exists.
		if child, ok := cur[segment]; ok {
			cur[segment] = encryptBodyPath(child, segments[1:], keyID)
		}
		return cur
	case []interface{}:
		segment := segments[0]
		// If the segment is "*", apply to all elements in the array.
		// Example: path ["body", "items", "*", "foo"] will encrypt all body.items[i].foo fields.
		if segment == "*" {
			for i, child := range cur {
				cur[i] = encryptBodyPath(child, segments[1:], keyID)
			}
			return cur
		}
		// If the segment is a number, apply to that index.
		// Example: path ["body", "items", "0", "foo"] will encrypt body.items[0].foo.
		if idx, err := strconv.Atoi(segment); err == nil {
			if idx >= 0 && idx < len(cur) {
				cur[idx] = encryptBodyPath(cur[idx], segments[1:], keyID)
			}
		}
		return cur
	default:
		// If not a map or array, return as is.
		return bodyNode
	}
}

// EncryptMessageBody encrypts the message body or fields according to the policy.
func EncryptMessageBody(m *models.Message, thread models.Thread) (interface{}, error) {
	tr := telemetry.Track("security.encrypt_message_body")
	defer tr.Finish()

	// If the message is nil, return an error.
	if m == nil {
		return nil, errors.New("nil message")
	}

	// If encryption is not enabled, return the body as is.
	if !EncryptionEnabled() {
		return m.Body, nil
	}

	// Get the key ID from the thread. Guard against missing KMS metadata.
	if thread.KMS == nil || thread.KMS.KeyID == "" {
		return nil, fmt.Errorf("encryption enabled but no DEK configured for thread %s", thread.ID)
	}
	keyID := thread.KMS.KeyID

	// If there is a field policy, encrypt only the specified fields.
	if EncryptionHasFieldPolicy() {
		tr.Mark("encrypt_fields")
		// Marshal the message to JSON.
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}

		// Require a KMS provider.
		if !kms.IsProviderEnabled() {
			return nil, errors.New("no kms provider registered")
		}

		// Unmarshal the message to a generic interface.
		var v interface{}
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}

		// For each field rule, encrypt the path.
		for _, rule := range fieldRules {
			v = encryptBodyPath(v, rule.segments, keyID)
		}

		// Marshal the result back to JSON.
		nb, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}

		// Unmarshal back to a Message struct to extract the body.
		var out models.Message
		if err := json.Unmarshal(nb, &out); err != nil {
			return nil, fmt.Errorf("failed to marshal message after field encryption: %w", err)
		}

		// Return the encrypted body.
		return out.Body, nil
	}

	// If no field policy, encrypt the entire body.
	if m.Body != nil {
		tr.Mark("encrypt_body")
		// Marshal the body to JSON.
		bodyBytes, err := json.Marshal(m.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal message body: %w", err)
		}
		// Encrypt the body using the KMS DEK.
		ct, _, err := kms.EncryptWithDEK(keyID, bodyBytes, nil)
		if err != nil {
			return nil, err
		}
		// Return the encrypted object.
		// Example: {"_enc": "gcm", "v": "<base64-ct>"}
		encBody := map[string]interface{}{"_enc": "gcm", "v": base64.StdEncoding.EncodeToString(ct)}
		return encBody, nil
	}
	// If body is nil, return as is.
	return m.Body, nil
}

// decryptBodyPath recursively decrypts the value at the given path.
func decryptBodyPath(value interface{}, segments []string, keyID string) (interface{}, error) {
	// If there are no more segments, try to decrypt the value if it's an encrypted object.
	if len(segments) == 0 {
		// At the target field, attempt to decrypt if it's an encrypted object.
		// Example: {"_enc": "gcm", "v": "<base64-ct>"}
		if m, ok := value.(map[string]interface{}); ok {
			if encType, ok := m["_enc"].(string); ok && encType == "gcm" {
				if sv, ok := m["v"].(string); ok {
					// Decode the base64 ciphertext.
					raw, err := base64.StdEncoding.DecodeString(sv)
					if err != nil {
						return value, fmt.Errorf("base64 decode failed: %w", err)
					}
					// Decrypt using the KMS DEK.
					pt, err := kms.DecryptWithDEK(keyID, raw, nil)
					if err != nil {
						return value, fmt.Errorf("kms decrypt failed: %w", err)
					}
					// Unmarshal the plaintext JSON.
					var out interface{}
					if err := json.Unmarshal(pt, &out); err != nil {
						return value, fmt.Errorf("json unmarshal failed: %w", err)
					}
					return out, nil
				}
			}
		}
		// If not encrypted, or decryption fails, return the value as is.
		return value, nil
	}

	// If there are more segments, traverse the structure.
	switch cur := value.(type) {
	case map[string]interface{}:
		segment := segments[0]
		// If the segment is "*", apply to all keys at this level.
		// Example: path ["body", "*", "foo"] will decrypt all body.<any>.foo fields.
		if segment == "*" {
			var firstErr error
			for k, child := range cur {
				res, err := decryptBodyPath(child, segments[1:], keyID)
				if err != nil && firstErr == nil {
					firstErr = err
				}
				cur[k] = res
			}
			return cur, firstErr
		}
		// Otherwise, descend into the named field if it exists.
		if child, ok := cur[segment]; ok {
			res, err := decryptBodyPath(child, segments[1:], keyID)
			cur[segment] = res
			return cur, err
		}
		return cur, nil
	case []interface{}:
		segment := segments[0]
		// If the segment is "*", apply to all elements in the array.
		// Example: path ["body", "items", "*", "foo"] will decrypt all body.items[i].foo fields.
		if segment == "*" {
			var firstErr error
			for i, child := range cur {
				res, err := decryptBodyPath(child, segments[1:], keyID)
				if err != nil && firstErr == nil {
					firstErr = err
				}
				cur[i] = res
			}
			return cur, firstErr
		}
		// If the segment is a number, apply to that index.
		// Example: path ["body", "items", "0", "foo"] will decrypt body.items[0].foo.
		if idx, err := strconv.Atoi(segment); err == nil {
			if idx >= 0 && idx < len(cur) {
				res, err := decryptBodyPath(cur[idx], segments[1:], keyID)
				cur[idx] = res
				return cur, err
			}
		}
		return cur, nil
	default:
		// If not a map or array, return as is.
		return value, nil
	}
}

// DecryptMessageBody decrypts the message body or fields according to the policy.
// Returns the decrypted message body (or the original body if not encrypted or not enabled).
func DecryptMessageBody(m *models.Message, threadKeyID string) (interface{}, error) {
	tr := telemetry.Track("security.decrypt_message_body")
	defer tr.Finish()

	// If the message is nil, return an error.
	if m == nil {
		return nil, errors.New("nil message")
	}

	// If encryption is not enabled, return the body as is.
	if !EncryptionEnabled() {
		return m.Body, nil
	}

	// Require a thread key ID.
	if threadKeyID == "" {
		return nil, errors.New("no thread key id provided")
	}

	// If there is a field policy, decrypt only the specified fields.
	if EncryptionHasFieldPolicy() {
		tr.Mark("decrypt_fields")
		// Marshal the message to JSON.
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}

		// Unmarshal the message to a generic interface.
		var v interface{}
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}

		var firstErr error
		// For each field rule, decrypt the path.
		for _, rule := range fieldRules {
			res, err := decryptBodyPath(v, rule.segments, threadKeyID)
			if err != nil && firstErr == nil {
				firstErr = err
			}
			v = res
		}

		// Marshal the result back to JSON.
		nb, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}

		// Unmarshal back to a Message struct to extract the body.
		var out models.Message
		if err := json.Unmarshal(nb, &out); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message after field decryption: %w", err)
		}

		// Return the decrypted body and any error encountered.
		return out.Body, firstErr
	}

	// If no field policy, decrypt the entire body if it is encrypted.
	if m.Body != nil {
		tr.Mark("decrypt_body")
		// If the body is an encrypted object, try to decrypt it.
		if mMap, ok := m.Body.(map[string]interface{}); ok {
			if encType, ok := mMap["_enc"].(string); ok && encType == "gcm" {
				if sv, ok := mMap["v"].(string); ok {
					raw, err := base64.StdEncoding.DecodeString(sv)
					if err != nil {
						logger.Warn("decrypt_message_body_base64_decode_failed", "error", err)
						return m.Body, fmt.Errorf("base64 decode failed: %w", err)
					}
					pt, err := kms.DecryptWithDEK(threadKeyID, raw, nil)
					if err != nil {
						logger.Warn("decrypt_message_body_decrypt_failed", "error", err)
						return m.Body, fmt.Errorf("kms decrypt failed: %w", err)
					}
					var out interface{}
					if err := json.Unmarshal(pt, &out); err != nil {
						logger.Warn("decrypt_message_body_unmarshal_failed", "error", err)
						return m.Body, fmt.Errorf("json unmarshal failed: %w", err)
					}
					return out, nil
				}
			}
		}
		// If not encrypted or decryption fails, return as is.
		return m.Body, nil
	}
	// If body is nil, return as is.
	return m.Body, nil
}
