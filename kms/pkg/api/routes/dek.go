package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/progressdb/kms/pkg/api"
	utils "github.com/progressdb/kms/pkg/api/utils"
)

type CreateDEKRequest struct {
	ThreadKey string `json:"thread_key"`
}

type CreateDEKResponse struct {
	KeyID      string `json:"key_id"`
	Wrapped    string `json:"wrapped"`
	KekID      string `json:"kek_id"`
	KekVersion string `json:"kek_version"`
}

func (d *Dependencies) CreateDEK(w http.ResponseWriter, r *http.Request) {
	if !d.Provider.Enabled() {
		api.WriteInternalError(w, "no provider configured")
		return
	}

	var req CreateDEKRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteBadRequest(w, "invalid request body")
		return
	}

	if err := utils.ValidateThreadKey(req.ThreadKey); err != nil {
		api.WriteBadRequest(w, err.Error())
		return
	}

	kid, wrapped, kekID, kekVer, err := d.Provider.CreateDEKForThread(req.ThreadKey)
	if err != nil {
		api.WriteInternalError(w, err.Error())
		return
	}

	meta := map[string]string{
		"wrapped":    base64.StdEncoding.EncodeToString(wrapped),
		"thread_key": req.ThreadKey,
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
		api.WriteBadRequest(w, "missing key_id")
		return
	}

	if err := utils.ValidateKey(keyID); err != nil {
		api.WriteBadRequest(w, err.Error())
		return
	}

	mb, err := d.Store.GetKeyMeta(keyID)
	if err != nil {
		api.WriteNotFound(w, "key not found")
		return
	}

	var m map[string]string
	if err := json.Unmarshal(mb, &m); err != nil {
		api.WriteInternalError(w, "invalid key metadata")
		return
	}

	response := GetWrappedResponse{
		Wrapped: m["wrapped"],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
