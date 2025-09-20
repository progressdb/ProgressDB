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
	"progressdb/pkg/models"
)

var key []byte
var keyLocked bool

type EncField struct {
	Path      string
	Algorithm string
}

type fieldRule struct {
	segs      []string
	algorithm string
}

var fieldRules []fieldRule

// SetKeyEncryptionHex sets the AES-256 master key (hex string). An empty string clears it.
func SetKeyEncryptionHex(hexKey string) error {
	if hexKey == "" {
		if key != nil && keyLocked {
			_ = UnlockMemory(key)
			keyLocked = false
		}
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
	if key != nil && keyLocked {
		_ = UnlockMemory(key)
		keyLocked = false
	}
	key = b
	if err := LockMemory(key); err == nil {
		keyLocked = true
	}
	return nil
}

// EncryptionEnabled reports whether encryption is available (provider or local key).
func EncryptionEnabled() bool {
	if kms.IsProviderEnabled() {
		return true
	}
	return key != nil && len(key) == 32
}

// SetEncryptionFieldPolicy configures selective field encryption paths.
// Only algorithm "aes-gcm" is supported for now.
func SetEncryptionFieldPolicy(fields []EncField) error {
	// Clear any existing field rules before setting new ones
	fieldRules = fieldRules[:0]

	// Iterate over each provided encryption field configuration
	for _, f := range fields {
		// Normalize and validate the algorithm (default to "aes-gcm" if empty)
		alg := strings.ToLower(strings.TrimSpace(f.Algorithm))
		if alg == "" {
			alg = "aes-gcm"
		}
		// Only "aes-gcm" is supported at this time
		if alg != "aes-gcm" {
			return fmt.Errorf("unsupported algorithm: %s", f.Algorithm)
		}

		// Trim and validate the field path
		p := strings.TrimSpace(f.Path)
		if p == "" {
			continue // Skip empty paths
		}

		// Split the path into segments for nested field access
		segs := strings.Split(p, ".")

		// Add the rule to the global fieldRules slice
		fieldRules = append(fieldRules, fieldRule{segs: segs, algorithm: alg})
	}
	// Return nil to indicate success
	return nil
}

// EncryptionHasFieldPolicy reports whether any field policy was configured.
func EncryptionHasFieldPolicy() bool { return len(fieldRules) > 0 }

func DecryptJSONFields(jsonBytes []byte, keyID string) ([]byte, error) {
	if !EncryptionEnabled() || !EncryptionHasFieldPolicy() {
		return append([]byte(nil), jsonBytes...), nil
	}
	if !kms.IsProviderEnabled() {
		return nil, errors.New("no kms provider registered")
	}
	if keyID == "" {
		return nil, errors.New("no thread key ID provided")
	}
	var v interface{}
	if err := json.Unmarshal(jsonBytes, &v); err != nil {
		return nil, err
	}
	v = decryptAll(v, keyID)
	out, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func decryptAll(node interface{}, keyID string) interface{} {
	switch cur := node.(type) {
	case map[string]interface{}:
		if encType, ok := cur["_enc"].(string); ok {
			if encType == "gcm" {
				if sv, ok := cur["v"].(string); ok {
					if raw, err := base64.StdEncoding.DecodeString(sv); err == nil {
						if pt, err := kms.DecryptWithDEK(keyID, raw, nil); err == nil {
							var out interface{}
							if err := json.Unmarshal(pt, &out); err == nil {
								return decryptAll(out, keyID)
							}
						}
					}
				}
			}
		}
		for k, v := range cur {
			cur[k] = decryptAll(v, keyID)
		}
		return cur
	case []interface{}:
		for i, v := range cur {
			cur[i] = decryptAll(v, keyID)
		}
		return cur
	default:
		return node
	}
}

func EncryptJSONFields(jsonBytes []byte, keyID string) ([]byte, error) {
	if !EncryptionEnabled() || !EncryptionHasFieldPolicy() {
		return append([]byte(nil), jsonBytes...), nil
	}
	if !kms.IsProviderEnabled() {
		return nil, errors.New("no kms provider registered")
	}
	if keyID == "" {
		return nil, errors.New("no thread key ID provided")
	}
	var v interface{}
	if err := json.Unmarshal(jsonBytes, &v); err != nil {
		return nil, err
	}
	for _, rule := range fieldRules {
		v = encryptBodyPath(v, rule.segs, keyID)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func encryptBodyPath(node interface{}, segs []string, keyID string) interface{} {
	// if there are no more segments, encrypt the current node
	// Example: If segs is [] (empty), and node is {"ssn": "123-45-6789"}, this will encrypt the whole object or value at this path.
	if len(segs) == 0 {
		raw, err := json.Marshal(node)
		if err != nil {
			return node
		}
		ct, _, err := kms.EncryptWithDEK(keyID, raw, nil)
		if err != nil {
			return node
		}
		return map[string]interface{}{"_enc": "gcm", "v": base64.StdEncoding.EncodeToString(ct)}
	}

	// handle the current node based on its type
	switch cur := node.(type) {
	case map[string]interface{}:
		// if the current segment is "*", apply recursively to all keys
		// Example: segs = ["*","password"], node = {"user1": {"password": "abc"}, "user2": {"password": "def"}}
		// This will encrypt the "password" field for every user in the map.
		seg := segs[0]
		if seg == "*" {
			for k, child := range cur {
				cur[k] = encryptBodyPath(child, segs[1:], keyID)
			}
			return cur
		}
		// otherwise, apply to the specific key if it exists
		// Example: segs = ["profile","email"], node = {"profile": {"email": "a@b.com"}}
		// This will encrypt the "email" field inside the "profile" object.
		if child, ok := cur[seg]; ok {
			cur[seg] = encryptBodyPath(child, segs[1:], keyID)
		}
		return cur
	case []interface{}:
		// if the current segment is "*", apply recursively to all elements
		// Example: segs = ["*","secret"], node = [{"secret": "a"}, {"secret": "b"}]
		// This will encrypt the "secret" field in every element of the array.
		seg := segs[0]
		if seg == "*" {
			for i, child := range cur {
				cur[i] = encryptBodyPath(child, segs[1:], keyID)
			}
			return cur
		}
		// if the segment is an integer, apply to the specific index
		// Example: segs = ["0","token"], node = [{"token": "abc"}, {"token": "def"}]
		// This will encrypt the "token" field in the first element of the array.
		if idx, err := strconv.Atoi(seg); err == nil {
			if idx >= 0 && idx < len(cur) {
				cur[idx] = encryptBodyPath(cur[idx], segs[1:], keyID)
			}
		}
		return cur
	default:
		// if the node is not a map or slice, return as is
		// Example: node is a string, number, or bool; nothing to encrypt at this path.
		return node
	}
}

func EncryptMessageBody(m *models.Message, thread models.Thread) (interface{}, error) {
	if m == nil {
		return nil, errors.New("nil message")
	}

	// If encryption is not enabled, do nothing.
	if !EncryptionEnabled() {
		return m.Body, nil
	}

	// Get the thread's dek ID for encryption.
	keyID := thread.KMS.KeyID
	if keyID == "" {
		return nil, fmt.Errorf("encryption enabled but no DEK configured for thread %s", thread.ID)
	}

	// If a field policy is configured, encrypt only the configured fields.
	if EncryptionHasFieldPolicy() {
		// marshal the message to json so we can work with it as a generic structure
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}

		// ensure a kms provider is available for encryption
		if !kms.IsProviderEnabled() {
			return nil, errors.New("no kms provider registered")
		}

		// unmarshal the json into an interface{} for manipulation
		var v interface{}
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}

		// for each configured field rule, encrypt the specified field path in the structure
		for _, rule := range fieldRules {
			v = encryptBodyPath(v, rule.segs, keyID)
		}

		// marshal the modified structure back to json
		nb, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}

		// unmarshal the json back into a Message struct to extract the encrypted body
		var out models.Message
		if err := json.Unmarshal(nb, &out); err != nil {
			return nil, fmt.Errorf("failed to marshal message after field encryption: %w", err)
		}

		// return the (possibly encrypted) body field
		return out.Body, nil
	}

	// Otherwise, encrypt the whole message.Body if present.
	if m.Body != nil {
		bodyBytes, err := json.Marshal(m.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal message body: %w", err)
		}
		ct, _, err := kms.EncryptWithDEK(keyID, bodyBytes, nil)
		if err != nil {
			return nil, err
		}
		encBody := map[string]interface{}{"_enc": "gcm", "v": base64.StdEncoding.EncodeToString(ct)}
		return encBody, nil
	}
	return m.Body, nil
}
func DecryptMessageBody(m *models.Message, threadKeyID string) error {
	if m == nil {
		return errors.New("nil message")
	}
	if !EncryptionEnabled() || !EncryptionHasFieldPolicy() {
		return nil
	}
	if threadKeyID == "" {
		return errors.New("no thread key id provided")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	nb, err := DecryptJSONFields(b, threadKeyID)
	if err != nil {
		return err
	}
	return json.Unmarshal(nb, m)
}
