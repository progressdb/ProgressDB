package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	validation "github.com/progressdb/kms/pkg/api/utils"
)

type CreateDEKRequest struct {
	ThreadID string `json:"thread_id"`
}

type CreateDEKResponse struct {
	KeyID      string `json:"key_id"`
	Wrapped    string `json:"wrapped"`
	KekID      string `json:"kek_id"`
	KekVersion string `json:"kek_version"`
}

func (d *Dependencies) CreateDEK(w http.ResponseWriter, r *http.Request) {
	if !d.Provider.Enabled() {
		http.Error(w, "no provider configured", http.StatusInternalServerError)
		return
	}

	var req CreateDEKRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := validation.ValidateThreadID(req.ThreadID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	kid, wrapped, kekID, kekVer, err := d.Provider.CreateDEKForThread(req.ThreadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	meta := map[string]string{
		"wrapped":   base64.StdEncoding.EncodeToString(wrapped),
		"thread_id": req.ThreadID,
	}
	mb, _ := json.Marshal(meta)
	_ = d.Store.SaveKeyMeta(kid, mb)

	response := CreateDEKResponse{
		KeyID:      kid,
		Wrapped:    base64.StdEncoding.EncodeToString(wrapped),
		KekID:      kekID,
		KekVersion: kekVer,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type GetWrappedRequest struct {
	KeyID string `json:"key_id"`
}

type GetWrappedResponse struct {
	Wrapped string `json:"wrapped"`
}

func (d *Dependencies) GetWrapped(w http.ResponseWriter, r *http.Request) {
	keyID := r.URL.Query().Get("key_id")
	if keyID == "" {
		http.Error(w, "missing key_id", http.StatusBadRequest)
		return
	}

	if err := validation.ValidateKeyID(keyID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mb, err := d.Store.GetKeyMeta(keyID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var m map[string]string
	if err := json.Unmarshal(mb, &m); err != nil {
		http.Error(w, "invalid key metadata", http.StatusInternalServerError)
		return
	}

	response := GetWrappedResponse{
		Wrapped: m["wrapped"],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
