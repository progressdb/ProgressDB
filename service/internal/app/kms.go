package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"progressdb/pkg/config"
	"progressdb/pkg/store/encryption/kms"
)

func (a *App) setupKMS(ctx context.Context) error {
	cfg := config.GetConfig()

	useEncryption := resolveEncryptionEnabled(cfg)
	if !useEncryption {
		log.Printf("kms: encryption disabled")
		return nil
	}

	masterKey, err := resolveMasterKey(cfg)
	if err != nil {
		return err
	}

	mode := resolveKMSMode(cfg)
	switch mode {
	case "embedded":
		return a.setupEmbeddedKMS(ctx, masterKey)
	case "external":
		return a.setupExternalKMS(cfg.Encryption.KMS.Endpoint)
	default:
		return fmt.Errorf("unknown KMS mode=%q; must be embedded or external", mode)
	}
}

func resolveEncryptionEnabled(cfg *config.Config) bool {
	return cfg.Encryption.Enabled
}

func resolveMasterKey(cfg *config.Config) (string, error) {
	var mk string
	switch {
	case strings.TrimSpace(cfg.Encryption.KMS.MasterKeyFile) != "":
		mkFile := strings.TrimSpace(cfg.Encryption.KMS.MasterKeyFile)
		keyb, err := os.ReadFile(mkFile)
		if err != nil {
			return "", fmt.Errorf("failed to read master key file %s: %w", mkFile, err)
		}
		mk = strings.TrimSpace(string(keyb))
	case strings.TrimSpace(cfg.Encryption.KMS.MasterKeyHex) != "":
		mk = strings.TrimSpace(cfg.Encryption.KMS.MasterKeyHex)
	default:
		return "", fmt.Errorf("encryption enabled but no master key provided in server config. Set encryption.kms.master_key_file or encryption.kms.master_key_hex")
	}
	if mk == "" {
		return "", fmt.Errorf("master key is empty")
	}
	if kb, err := hex.DecodeString(mk); err != nil || len(kb) != 32 {
		return "", fmt.Errorf("invalid master_key_hex: must be 64-hex (32 bytes)")
	}
	return mk, nil
}

func resolveKMSMode(cfg *config.Config) string {
	mode := strings.ToLower(strings.TrimSpace(cfg.Encryption.KMS.Mode))
	if mode == "" {
		return "embedded" // fallback to default
	}
	return mode
}

func (a *App) setupEmbeddedKMS(ctx context.Context, masterKey string) error {
	if err := kms.RegisterHashicorpEmbeddedProvider(ctx, masterKey); err != nil {
		return fmt.Errorf("failed to initialize embedded KMS provider: %w", err)
	}
	log.Printf("encryption enabled: true (embedded mode, hashicorp AEAD)")
	return nil
}

func (a *App) setupExternalKMS(kmsEndpoint string) error {
	a.rc = kms.NewRemoteClient(kmsEndpoint)
	kms.RegisterKMSProvider(a.rc)
	if err := a.rc.Health(); err != nil {
		return fmt.Errorf("KMS health check failed at %s: %w; ensure KMS is installed and reachable", kmsEndpoint, err)
	}
	log.Printf("encryption enabled: true (external KMS endpoint=%s)", kmsEndpoint)
	return nil
}
