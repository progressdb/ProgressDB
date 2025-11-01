package handlers

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/progressdb/kms/pkg/api"
	utils "github.com/progressdb/kms/pkg/api/utils"
)

type DecryptRequest struct {
	KeyID      string `json:"key_id"`
	Ciphertext string `json:"ciphertext"`
}

type DecryptResponse struct {
	Plaintext string `json:"plaintext"`
}

func (d *Dependencies) Decrypt(w http.ResponseWriter, r *http.Request) {
	var req DecryptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteBadRequest(w, "invalid request body")
		return
	}

	if err := utils.ValidateKeyID(req.KeyID); err != nil {
		api.WriteBadRequest(w, err.Error())
		return
	}

	if err := utils.ValidateBase64(req.Ciphertext); err != nil {
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

	if pt, err := d.Provider.DecryptWithDEK(req.KeyID, mustDecodeBase64(req.Ciphertext), nil); err == nil {
		response := DecryptResponse{
			Plaintext: base64.StdEncoding.EncodeToString(pt),
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

		pt, err := decryptWithRaw(dek, req.Ciphertext)
		if err != nil {
			api.WriteInternalError(w, err.Error())
			return
		}

		response := DecryptResponse{
			Plaintext: string(pt),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	api.WriteInternalError(w, "decryption not supported")
}

func decryptWithRaw(dek []byte, b64 string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce := data[:ns]
	ct := data[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}
