package api

import (
	"encoding/json"
	"net/http"

	kmsinterface "github.com/progressdb/kms/pkg/interface"
)

// Error codes for standardized error responses
const (
	ErrCodeInvalidRequest     = "INVALID_REQUEST"
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeUnauthorized       = "UNAUTHORIZED"
	ErrCodeInternalError      = "INTERNAL_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	ErrCodeKeyNotFound        = "KEY_NOT_FOUND"
	ErrCodeInvalidKey         = "INVALID_KEY"
	ErrCodeEncryptionFailed   = "ENCRYPTION_FAILED"
	ErrCodeDecryptionFailed   = "DECRYPTION_FAILED"
)

// WriteErrorResponse writes a standardized error response
func WriteErrorResponse(w http.ResponseWriter, code string, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := kmsinterface.ErrorResponse{
		Error: message,
		Code:  code,
	}

	json.NewEncoder(w).Encode(response)
}

// WriteBadRequest writes a bad request error
func WriteBadRequest(w http.ResponseWriter, message string) {
	WriteErrorResponse(w, ErrCodeInvalidRequest, message, http.StatusBadRequest)
}

// WriteNotFound writes a not found error
func WriteNotFound(w http.ResponseWriter, message string) {
	WriteErrorResponse(w, ErrCodeNotFound, message, http.StatusNotFound)
}

// WriteInternalError writes an internal server error
func WriteInternalError(w http.ResponseWriter, message string) {
	WriteErrorResponse(w, ErrCodeInternalError, message, http.StatusInternalServerError)
}

// WriteServiceUnavailable writes a service unavailable error
func WriteServiceUnavailable(w http.ResponseWriter, message string) {
	WriteErrorResponse(w, ErrCodeServiceUnavailable, message, http.StatusServiceUnavailable)
}
