package http

import (
	"encoding/json"
	"net/http"

	"github.com/progressdb/kms/pkg/kms"
)

// Server provides HTTP API for KMS operations (external mode)
type Server struct {
	kms  *kms.KMS
	addr string
}

// NewServer creates a new HTTP server for KMS
func NewServer(kmsInstance *kms.KMS, addr string) *Server {
	return &Server{
		kms:  kmsInstance,
		addr: addr,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	router := http.NewServeMux()

	// Register only the 3 core operation routes
	router.HandleFunc("/deks", s.handleCreateDEK)
	router.HandleFunc("/encrypt", s.handleEncrypt)
	router.HandleFunc("/decrypt", s.handleDecrypt)

	return http.ListenAndServe(s.addr, router)
}

// Create DEK endpoint
func (s *Server) handleCreateDEK(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		KeyID string `json:"key_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// If request body is empty or invalid, proceed without key_id
		req.KeyID = ""
	}

	var dek *kms.DEK
	var err error

	if req.KeyID != "" {
		dek, err = s.kms.CreateDEK(req.KeyID)
	} else {
		dek, err = s.kms.CreateDEK()
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(dek)
}

// Encrypt endpoint
func (s *Server) handleEncrypt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		KeyID     string `json:"key_id"`
		Plaintext []byte `json:"plaintext"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ciphertext, err := s.kms.Encrypt(req.KeyID, req.Plaintext)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ciphertext": ciphertext,
	})
}

// Decrypt endpoint
func (s *Server) handleDecrypt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		KeyID      string `json:"key_id"`
		Ciphertext []byte `json:"ciphertext"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	plaintext, err := s.kms.Decrypt(req.KeyID, req.Ciphertext)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"plaintext": plaintext,
	})
}
