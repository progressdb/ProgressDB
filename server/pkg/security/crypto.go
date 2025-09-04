package security

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "encoding/hex"
    "errors"
    "fmt"
    "io"
    "strconv"
    "strings"
    "unicode/utf8"

    "encoding/json"
)

var key []byte

type EncField struct {
    Path      string
    Algorithm string
}

type fieldRule struct {
    segs      []string
    algorithm string
}

var fieldRules []fieldRule

// SetFieldPolicy configures selective field encryption paths.
// Only algorithm "aes-gcm" is supported for now.
func SetFieldPolicy(fields []EncField) error {
    fieldRules = fieldRules[:0]
    for _, f := range fields {
        alg := strings.ToLower(strings.TrimSpace(f.Algorithm))
        if alg == "" {
            alg = "aes-gcm"
        }
        if alg != "aes-gcm" {
            return fmt.Errorf("unsupported algorithm: %s", f.Algorithm)
        }
        p := strings.TrimSpace(f.Path)
        if p == "" {
            continue
        }
        segs := strings.Split(p, ".")
        fieldRules = append(fieldRules, fieldRule{segs: segs, algorithm: alg})
    }
    return nil
}

// HasFieldPolicy returns true if selective field encryption is configured.
func HasFieldPolicy() bool { return len(fieldRules) > 0 }

// SetKeyHex sets the AES-256-GCM key from a hex string.
func SetKeyHex(hexKey string) error {
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

// Enabled returns true if encryption key is configured.
func Enabled() bool { return len(key) == 32 }

// Encrypt returns nonce|ciphertext using AES-256-GCM.
func Encrypt(plaintext []byte) ([]byte, error) {
    if !Enabled() {
        // No-op: return copy of plaintext
        out := append([]byte(nil), plaintext...)
        return out, nil
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, err
    }
    out := gcm.Seal(nil, nonce, plaintext, nil)
    // Prepend nonce for storage
    return append(nonce, out...), nil
}

// Decrypt expects nonce|ciphertext.
func Decrypt(data []byte) ([]byte, error) {
    if !Enabled() {
        return append([]byte(nil), data...), nil
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    ns := gcm.NonceSize()
    if len(data) < ns {
        return nil, errors.New("ciphertext too short")
    }
    nonce := data[:ns]
    ct := data[ns:]
    return gcm.Open(nil, nonce, ct, nil)
}

// envelope represents an encrypted JSON field value.
type envelope struct {
    Enc string `json:"_enc"`
    V   string `json:"v"`
}

// EncryptJSONFields encrypts configured JSON paths within the provided JSON bytes.
// Returns the modified JSON if parsing/encryption succeeds.
func EncryptJSONFields(jsonBytes []byte) ([]byte, error) {
    if !Enabled() || !HasFieldPolicy() {
        return append([]byte(nil), jsonBytes...), nil
    }
    // Quick sanity: must look like JSON object or array
    if !looksLikeJSON(jsonBytes) {
        return nil, errors.New("not json")
    }
    var v interface{}
    if err := json.Unmarshal(jsonBytes, &v); err != nil {
        return nil, err
    }
    for _, rule := range fieldRules {
        v = encryptAt(v, rule.segs)
    }
    out, err := json.Marshal(v)
    if err != nil {
        return nil, err
    }
    return out, nil
}

// DecryptJSONFields decrypts any envelope objects found in JSON.
func DecryptJSONFields(jsonBytes []byte) ([]byte, error) {
    if !Enabled() || !HasFieldPolicy() {
        return append([]byte(nil), jsonBytes...), nil
    }
    if !looksLikeJSON(jsonBytes) {
        return nil, errors.New("not json")
    }
    var v interface{}
    if err := json.Unmarshal(jsonBytes, &v); err != nil {
        return nil, err
    }
    v = decryptAll(v)
    out, err := json.Marshal(v)
    if err != nil {
        return nil, err
    }
    return out, nil
}

func looksLikeJSON(b []byte) bool {
    s := strings.TrimSpace(string(b))
    if s == "" {
        return false
    }
    r, _ := utf8.DecodeRuneInString(s)
    return r == '{' || r == '['
}

func encryptAt(node interface{}, segs []string) interface{} {
    if len(segs) == 0 {
        // Encrypt current node value as JSON bytes and wrap in envelope.
        raw, err := json.Marshal(node)
        if err != nil {
            return node
        }
        ct, err := Encrypt(raw)
        if err != nil {
            return node
        }
        return map[string]interface{}{
            "_enc": "gcm",
            "v":    base64.StdEncoding.EncodeToString(ct),
        }
    }
    switch cur := node.(type) {
    case map[string]interface{}:
        seg := segs[0]
        if seg == "*" {
            for k, child := range cur {
                cur[k] = encryptAt(child, segs[1:])
            }
            return cur
        }
        if child, ok := cur[seg]; ok {
            cur[seg] = encryptAt(child, segs[1:])
        }
        return cur
    case []interface{}:
        seg := segs[0]
        if seg == "*" {
            for i, child := range cur {
                cur[i] = encryptAt(child, segs[1:])
            }
            return cur
        }
        if idx, err := strconv.Atoi(seg); err == nil {
            if idx >= 0 && idx < len(cur) {
                cur[idx] = encryptAt(cur[idx], segs[1:])
            }
        }
        return cur
    default:
        return node
    }
}

func decryptAll(node interface{}) interface{} {
    switch cur := node.(type) {
    case map[string]interface{}:
        // Check for envelope directly
        if encType, ok := cur["_enc"].(string); ok {
            if encType == "gcm" {
                if sv, ok := cur["v"].(string); ok {
                    if raw, err := base64.StdEncoding.DecodeString(sv); err == nil {
                        if pt, err := Decrypt(raw); err == nil {
                            // Replace with parsed JSON
                            var out interface{}
                            if err := json.Unmarshal(pt, &out); err == nil {
                                return decryptAll(out)
                            }
                        }
                    }
                }
            }
        }
        // Recurse into map
        for k, v := range cur {
            cur[k] = decryptAll(v)
        }
        return cur
    case []interface{}:
        for i, v := range cur {
            cur[i] = decryptAll(v)
        }
        return cur
    default:
        return node
    }
}
