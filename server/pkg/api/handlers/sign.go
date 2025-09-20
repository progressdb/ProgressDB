package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"progressdb/pkg/logger"
)

// RegisterSigning registers the signing endpoint onto the provided router.
// This endpoint is protected by the existing security middleware (backend API keys)
// and will use the caller's API key value as the signing secret.
func RegisterSigning(r *mux.Router) {
	r.HandleFunc("/_sign", signHandler).Methods(http.MethodPost)
}

// signHandler handles POST requests to the /_sign endpoint.
// It generates an HMAC-SHA256 signature for a given userId using the caller's API key as the secret.
// Only requests with the "backend" role are permitted. The API key is extracted from the
// Authorization (Bearer) or X-API-Key header. The request body must be JSON with a "userId" field.
// On success, responds with a JSON object containing the userId and the computed signature.
func signHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Log the request (no sensitive content)
	logger.InfoKV("signHandler called", "remote", r.RemoteAddr, "path", r.URL.Path)

	// only backend roles may request signatures
	role := r.Header.Get("X-Role-Name")
	if role != "backend" {
		logger.WarnKV("forbidden: non-backend role attempted to sign", "role", role, "remote", r.RemoteAddr)
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
		logger.WarnKV("missing api key in signHandler", "remote", r.RemoteAddr)
		http.Error(w, `{"error":"missing api key"}`, http.StatusUnauthorized)
		return
	}

	var payload struct {
		UserID string `json:"userId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.UserID == "" {
		logger.WarnKV("invalid payload in signHandler", "error", err, "remote", r.RemoteAddr)
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	// Log the signing attempt (do not log userId or key)
	logger.InfoKV("signing userId", "remote", r.RemoteAddr, "role", role)

	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload.UserID))
	sig := hex.EncodeToString(mac.Sum(nil))
	if err := json.NewEncoder(w).Encode(map[string]string{"userId": payload.UserID, "signature": sig}); err != nil {
		logger.ErrorKV("failed to encode signHandler response", "error", err, "remote", r.RemoteAddr)
	}
}
