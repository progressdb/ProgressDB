package utils

import (
    "encoding/json"
    "net/http"
)

// JSONError writes a JSON error response with the given status code and
// message. It ensures the Content-Type is set to application/json.
func JSONError(w http.ResponseWriter, status int, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// JSONWrite writes the provided value as JSON with the given status code.
func JSONWrite(w http.ResponseWriter, status int, v interface{}) error {
    w.Header().Set("Content-Type", "application/json")
    if status != 0 {
        w.WriteHeader(status)
    }
    return json.NewEncoder(w).Encode(v)
}

