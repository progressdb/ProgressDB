package http

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/progressdb/kms/pkg/kms"
)

type Server struct {
	kms  *kms.KMS
	addr string
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func NewServer(kmsInstance *kms.KMS, addr string) *Server {
	return &Server{
		kms:  kmsInstance,
		addr: addr,
	}
}

func (s *Server) Start() error {
	router := http.NewServeMux()

	// Add logging middleware
	loggingRouter := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("KMS Request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Create a response writer wrapper to capture status code
		wrapper := &responseWriter{ResponseWriter: w, statusCode: 200}

		router.ServeHTTP(wrapper, r)

		duration := time.Since(start)
		log.Printf("KMS Response: %s %s - %d (%v)", r.Method, r.URL.Path, wrapper.statusCode, duration)
	})

	router.HandleFunc("/healthz", s.handleHealth)
	router.HandleFunc("/deks", s.handleCreateDEK)
	router.HandleFunc("/encrypt", s.handleEncrypt)
	router.HandleFunc("/decrypt", s.handleDecrypt)

	log.Printf("KMS server starting on %s", s.addr)
	return http.ListenAndServe(s.addr, loggingRouter)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleCreateDEK(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		KeyID string `json:"key_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
