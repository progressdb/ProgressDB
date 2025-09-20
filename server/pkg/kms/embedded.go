package kms

import (
	"context"

	kmss "github.com/progressdb/kms/pkg/security"
)

// NewHashicorpEmbeddedProvider constructs a HashiCorp AEAD-backed provider
// from the provided master key hex string and returns it directly. The
// server expects the provider to implement the shared KMSProvider alias.
func NewHashicorpEmbeddedProvider(ctx context.Context, masterHex string) (KMSProvider, error) {
	return kmss.NewHashicorpProviderFromHex(ctx, masterHex)
}

// RegisterHashicorpEmbeddedProvider constructs and registers an embedded
// HashiCorp provider using the provided master key hex. It returns any
// error encountered during construction or registration.
func RegisterHashicorpEmbeddedProvider(ctx context.Context, masterHex string) error {
	prov, err := NewHashicorpEmbeddedProvider(ctx, masterHex)
	if err != nil {
		return err
	}
	RegisterKMSProvider(prov)
	return nil
}
