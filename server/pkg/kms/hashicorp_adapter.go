package kms

import (
	"context"

	kmss "github.com/ha-sante/ProgressDB/kms/pkg/security"
	"progressdb/pkg/kmsapi"
)

// NewHashicorpEmbeddedProvider constructs a HashiCorp AEAD-backed provider
// from the provided master key hex string. The returned value implements
// the server's `kmsapi.KMSProvider` interface.
func NewHashicorpEmbeddedProvider(ctx context.Context, masterHex string) (kmsapi.KMSProvider, error) {
	p, err := kmss.NewHashicorpProviderFromHex(ctx, masterHex)
	if err != nil {
		return nil, err
	}
	// p is an interface value whose dynamic concrete type implements the
	// same method set; return it as the kmsapi.KMSProvider expected by the
	// server security package.
	return p, nil
}
