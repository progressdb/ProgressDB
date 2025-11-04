package kms

import (
	"context"

	kmss "github.com/progressdb/kms/pkg/core"
)

func NewHashicorpEmbeddedProvider(ctx context.Context, masterHex string, storePath string) (KMSProvider, error) {
	return kmss.NewHashicorpProviderFromHex(ctx, masterHex, storePath)
}

func RegisterHashicorpEmbeddedProvider(ctx context.Context, masterHex string, storePath string) error {
	prov, err := NewHashicorpEmbeddedProvider(ctx, masterHex, storePath)
	if err != nil {
		return err
	}
	RegisterKMSProvider(prov)
	return nil
}
