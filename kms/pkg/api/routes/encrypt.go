package handlers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/progressdb/kms/pkg/api"
	utils "github.com/progressdb/kms/pkg/api/utils"
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
		api.WriteBadRequest(w, "invalid request body")
		return
	}

	if err := utils.ValidateKeyID(req.KeyID); err != nil {
		api.WriteBadRequest(w, err.Error())
		return
	}

	if err := utils.ValidatePlaintext(req.Plaintext); err != nil {
		api.WriteBadRequest(w, err.Error())
		return
	}

	mb, err := d.Store.GetKeyMeta(req.KeyID)
	if err != nil {
		api.WriteNotFound(w, "key not found")
		return
	}

	var m map[string]string
	if err := json.Unmarshal(mb, &m); err != nil {
		api.WriteInternalError(w, "invalid key metadata")
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
		api.WriteInternalError(w, "invalid wrapped key")
		return
	}

	if u, ok := d.Provider.(interface{ UnwrapDEK([]byte) ([]byte, error) }); ok {
		dek, err := u.UnwrapDEK(wrappedB)
		if err != nil {
			api.WriteInternalError(w, err.Error())
			return
		}

		ct, err := encryptWithRawDEK(dek, mustDecodeBase64(req.Plaintext))
		if err != nil {
			api.WriteInternalError(w, err.Error())
			return
		}

		response := EncryptResponse{
			Ciphertext: ct,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	api.WriteInternalError(w, "encryption not supported")
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
