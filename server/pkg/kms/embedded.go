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
