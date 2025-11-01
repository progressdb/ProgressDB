package handlers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"

	validation "github.com/progressdb/kms/pkg/api/utils"
)

type EncryptRequest struct {
	KeyID     string `json:"key_id"`
	Plaintext string `json:"plaintext"`
}

type EncryptResponse struct {
	Ciphertext string `json:"ciphertext"`
	KeyVersion string `json:"key_version,omitempty"`
}

func (d *Dependencies) Encrypt(w http.ResponseWriter, r *http.Request) {
	var req EncryptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := validation.ValidateKeyID(req.KeyID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := validation.ValidatePlaintext(req.Plaintext); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mb, err := d.Store.GetKeyMeta(req.KeyID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var m map[string]string
	if err := json.Unmarshal(mb, &m); err != nil {
		http.Error(w, "invalid key metadata", http.StatusInternalServerError)
		return
	}

	if ct, kv, err := d.Provider.EncryptWithDEK(req.KeyID, mustDecodeBase64(req.Plaintext), nil); err == nil {
		response := EncryptResponse{
			Ciphertext: base64.StdEncoding.EncodeToString(ct),
			KeyVersion: kv,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	wrappedB, err := base64.StdEncoding.DecodeString(m["wrapped"])
	if err != nil {
		http.Error(w, "invalid wrapped key", http.StatusInternalServerError)
		return
	}

	if u, ok := d.Provider.(interface{ UnwrapDEK([]byte) ([]byte, error) }); ok {
		dek, err := u.UnwrapDEK(wrappedB)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		ct, err := encryptWithRawDEK(dek, mustDecodeBase64(req.Plaintext))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := EncryptResponse{
			Ciphertext: ct,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	http.Error(w, "encryption not supported", http.StatusInternalServerError)
}

func encryptWithRawDEK(dek, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}
