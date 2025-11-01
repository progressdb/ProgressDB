package kms

import (
	"context"

	kmss "github.com/progressdb/kms/pkg/core"
)

func NewHashicorpEmbeddedProvider(ctx context.Context, masterHex string) (KMSProvider, error) {
	return kmss.NewHashicorpProviderFromHex(ctx, masterHex)
}

func RegisterHashicorpEmbeddedProvider(ctx context.Context, masterHex string) error {
	prov, err := NewHashicorpEmbeddedProvider(ctx, masterHex)
	if err != nil {
		return err
	}
	RegisterKMSProvider(prov)
	return nil
}
