package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// RegisterSigning registers the signing endpoint onto the provided router.
// This endpoint is protected by the existing security middleware (backend API keys)
// and will use the caller's API key value as the signing secret.
func RegisterSigning(r *mux.Router) {
	r.HandleFunc("/_sign", signHandler).Methods(http.MethodPost)
}

func signHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// only backend roles may request signatures
	if role := r.Header.Get("X-Role-Name"); role != "backend" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	// determine the API key used by reading Authorization or X-API-Key header
	auth := r.Header.Get("Authorization")
	var key string
	if len(auth) > 7 && (auth[:7] == "Bearer " || auth[:7] == "bearer ") {
		key = auth[7:]
	}
	if key == "" {
		key = r.Header.Get("X-API-Key")
	}
	if key == "" {
		http.Error(w, `{"error":"missing api key"}`, http.StatusUnauthorized)
		return
	}

	var payload struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.UserID == "" {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload.UserID))
	sig := hex.EncodeToString(mac.Sum(nil))

	_ = json.NewEncoder(w).Encode(map[string]string{"userId": payload.UserID, "signature": sig})
}
