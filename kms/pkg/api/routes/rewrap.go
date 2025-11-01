package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/progressdb/kms/pkg/api"
	utils "github.com/progressdb/kms/pkg/api/utils"
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
		api.WriteBadRequest(w, "invalid request body")
		return
	}

	if err := utils.ValidateKey(req.KeyID); err != nil {
		api.WriteBadRequest(w, err.Error())
		return
	}

	if err := utils.ValidateHexKey(req.NewKEKHex); err != nil {
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

	newWrapped, newKekID, newKekVersion, err := d.Provider.RewrapDEKForThread(req.KeyID, req.NewKEKHex)
	if err != nil {
		api.WriteInternalError(w, err.Error())
		return
	}

	meta := map[string]string{
		"wrapped":    base64.StdEncoding.EncodeToString(newWrapped),
		"thread_key": m["thread_key"],
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
