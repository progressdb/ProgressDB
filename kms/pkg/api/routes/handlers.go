package handlers

import (
	"encoding/base64"

	security "github.com/progressdb/kms/pkg/core"
	"github.com/progressdb/kms/pkg/store"
)

type Dependencies struct {
	Provider security.KMSProvider
	Store    *store.Store
}

func mustDecodeBase64(s string) []byte {
	if s == "" {
		return nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b
	}
	return []byte(s)
}
