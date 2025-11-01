package handlers

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

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
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := utils.ValidateKeyID(req.KeyID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := utils.ValidateBase64(req.Ciphertext); err != nil {
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
		http.Error(w, "invalid wrapped key", http.StatusInternalServerError)
		return
	}

	if u, ok := d.Provider.(interface{ UnwrapDEK([]byte) ([]byte, error) }); ok {
		dek, err := u.UnwrapDEK(wrappedB)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		pt, err := decryptWithRaw(dek, req.Ciphertext)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := DecryptResponse{
			Plaintext: string(pt),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	http.Error(w, "decryption not supported", http.StatusInternalServerError)
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
