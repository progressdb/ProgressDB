package encryption

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/state/telemetry"
)

var key []byte

func SetKeyEncryptionHex(hexKey string) error {
	if hexKey == "" {
		key = nil
		return nil
	}
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return err
	}
	if l := len(b); l != 32 {
		return errors.New("encryption key must be 32 bytes (AES-256)")
	}
	key = b
	return nil
}

func EncryptionEnabled() bool {
	if IsProviderEnabled() {
		return true
	}
	return len(key) == 32
}

func encryptBodyPath(bodyNode interface{}, segments []string, keyID string) interface{} {
	if len(segments) == 0 {
		raw, err := json.Marshal(bodyNode)
		if err != nil {
			return bodyNode
		}
		ct, _, err := EncryptWithDEK(keyID, raw, nil)
		if err != nil {
			return bodyNode
		}
		return map[string]interface{}{"_enc": "gcm", "v": base64.StdEncoding.EncodeToString(ct)}
	}

	switch cur := bodyNode.(type) {
	case map[string]interface{}:
		segment := segments[0]
		if segment == "*" {
			for k, child := range cur {
				cur[k] = encryptBodyPath(child, segments[1:], keyID)
			}
			return cur
		}
		if child, ok := cur[segment]; ok {
			cur[segment] = encryptBodyPath(child, segments[1:], keyID)
		}
		return cur
	case []interface{}:
		segment := segments[0]
		if segment == "*" {
			for i, child := range cur {
				cur[i] = encryptBodyPath(child, segments[1:], keyID)
			}
			return cur
		}
		if idx, err := strconv.Atoi(segment); err == nil {
			if idx >= 0 && idx < len(cur) {
				cur[idx] = encryptBodyPath(cur[idx], segments[1:], keyID)
			}
		}
		return cur
	default:
		return bodyNode
	}
}

func EncryptMessageBody(m *models.Message, thread models.Thread) (interface{}, error) {
	tr := telemetry.Track("security.encrypt_message_body")
	defer tr.Finish()

	if m == nil {
		return nil, errors.New("nil message")
	}

	if !EncryptionEnabled() {
		return m.Body, nil
	}

	if thread.KMS == nil || thread.KMS.KeyID == "" {
		return nil, fmt.Errorf("encryption enabled but no DEK configured for thread %s", thread.Key)
	}
	keyID := thread.KMS.KeyID

	if EncryptionHasFieldPolicy() {
		tr.Mark("encrypt_fields")
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}

		if !IsProviderEnabled() {
			return nil, errors.New("no kms provider registered")
		}

		var v interface{}
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}

		for _, rule := range fieldRules {
			v = encryptBodyPath(v, rule.segments, keyID)
		}

		nb, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}

		var out models.Message
		if err := json.Unmarshal(nb, &out); err != nil {
			return nil, fmt.Errorf("failed to marshal message after field encryption: %w", err)
		}

		return out.Body, nil
	}

	if m.Body != nil {
		tr.Mark("encrypt_body")
		bodyBytes, err := json.Marshal(m.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal message body: %w", err)
		}
		ct, _, err := EncryptWithDEK(keyID, bodyBytes, nil)
		if err != nil {
			return nil, err
		}
		encBody := map[string]interface{}{"_enc": "gcm", "v": base64.StdEncoding.EncodeToString(ct)}
		return encBody, nil
	}
	return m.Body, nil
}

func decryptBodyPath(value interface{}, segments []string, keyID string) (interface{}, error) {
	if len(segments) == 0 {
		if m, ok := value.(map[string]interface{}); ok {
			if encType, ok := m["_enc"].(string); ok && encType == "gcm" {
				if sv, ok := m["v"].(string); ok {
					raw, err := base64.StdEncoding.DecodeString(sv)
					if err != nil {
						return value, fmt.Errorf("base64 decode failed: %w", err)
					}
					pt, err := DecryptWithDEK(keyID, raw, nil)
					if err != nil {
						return value, fmt.Errorf("kms decrypt failed: %w", err)
					}
					var out interface{}
					if err := json.Unmarshal(pt, &out); err != nil {
						return value, fmt.Errorf("json unmarshal failed: %w", err)
					}
					return out, nil
				}
			}
		}
		return value, nil
	}

	switch cur := value.(type) {
	case map[string]interface{}:
		segment := segments[0]
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
		if child, ok := cur[segment]; ok {
			res, err := decryptBodyPath(child, segments[1:], keyID)
			cur[segment] = res
			return cur, err
		}
		return cur, nil
	case []interface{}:
		segment := segments[0]
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
		if idx, err := strconv.Atoi(segment); err == nil {
			if idx >= 0 && idx < len(cur) {
				res, err := decryptBodyPath(cur[idx], segments[1:], keyID)
				cur[idx] = res
				return cur, err
			}
		}
		return cur, nil
	default:
		return value, nil
	}
}

func DecryptMessageBody(m *models.Message, threadKeyID string) (interface{}, error) {
	tr := telemetry.Track("security.decrypt_message_body")
	defer tr.Finish()

	if m == nil {
		return nil, errors.New("nil message")
	}

	if !EncryptionEnabled() {
		return m.Body, nil
	}

	if threadKeyID == "" {
		return nil, errors.New("no thread key id provided")
	}

	if EncryptionHasFieldPolicy() {
		tr.Mark("decrypt_fields")
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}

		var v interface{}
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}

		var firstErr error
		for _, rule := range fieldRules {
			res, err := decryptBodyPath(v, rule.segments, threadKeyID)
			if err != nil && firstErr == nil {
				firstErr = err
			}
			v = res
		}

		nb, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}

		var out models.Message
		if err := json.Unmarshal(nb, &out); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message after field decryption: %w", err)
		}

		return out.Body, firstErr
	}

	if m.Body != nil {
		tr.Mark("decrypt_body")
		if mMap, ok := m.Body.(map[string]interface{}); ok {
			if encType, ok := mMap["_enc"].(string); ok && encType == "gcm" {
				if sv, ok := mMap["v"].(string); ok {
					raw, err := base64.StdEncoding.DecodeString(sv)
					if err != nil {
						logger.Warn("decrypt_message_body_base64_decode_failed", "error", err)
						return m.Body, fmt.Errorf("base64 decode failed: %w", err)
					}
					pt, err := DecryptWithDEK(threadKeyID, raw, nil)
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
		return m.Body, nil
	}
	return m.Body, nil
}
