package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	validation "github.com/progressdb/kms/pkg/api/utils"
)

type RewrapRequest struct {
	KeyID     string `json:"key_id"`
	NewKEKHex string `json:"new_kek_hex"`
}

type RewrapResponse struct {
	NewWrapped    string `json:"new_wrapped"`
	NewKekID      string `json:"new_kek_id"`
	NewKekVersion string `json:"new_kek_version"`
}

func (d *Dependencies) Rewrap(w http.ResponseWriter, r *http.Request) {
	var req RewrapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := validation.ValidateKeyID(req.KeyID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := validation.ValidateHexKey(req.NewKEKHex); err != nil {
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

	newWrapped, newKekID, newKekVersion, err := d.Provider.RewrapDEKForThread(req.KeyID, req.NewKEKHex)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	meta := map[string]string{
		"wrapped":   base64.StdEncoding.EncodeToString(newWrapped),
		"thread_id": m["thread_id"],
	}
	mb, _ = json.Marshal(meta)
	_ = d.Store.SaveKeyMeta(req.KeyID, mb)

	response := RewrapResponse{
		NewWrapped:    base64.StdEncoding.EncodeToString(newWrapped),
		NewKekID:      newKekID,
		NewKekVersion: newKekVersion,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
