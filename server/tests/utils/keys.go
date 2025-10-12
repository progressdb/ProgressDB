package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const (
	BackendAPIKey  = "backend-test"
	FrontendAPIKey = "frontend-test"
	AdminAPIKey    = "admin-test"
	SigningSecret  = "signsecret"
)

// SignHMAC returns hex HMAC-SHA256 of user using key.
func SignHMAC(key, user string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(user))
	return hex.EncodeToString(mac.Sum(nil))
}
