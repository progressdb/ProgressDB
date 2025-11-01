package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	kmsconfig "github.com/progressdb/kms/pkg/config"
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
	// Convert service config to KMS config format
	kmsCfg := &kmsconfig.Config{
		Encryption: kmsconfig.EncryptionConfig{
			KMS: struct {
				Mode          string `yaml:"mode,default=embedded"`
				Endpoint      string `yaml:"endpoint,default=127.0.0.1:6820"`
				DataDir       string `yaml:"data_dir,default=/kms"`
				Binary        string `yaml:"binary,default=/usr/local/bin/progressdb-kms"`
				MasterKeyFile string `yaml:"master_key_file"`
				MasterKeyHex  string `yaml:"master_key_hex"`
			}{
				Mode:          cfg.Encryption.KMS.Mode,
				Endpoint:      cfg.Encryption.KMS.Endpoint,
				DataDir:       cfg.Encryption.KMS.DataDir,
				Binary:        cfg.Encryption.KMS.Binary,
				MasterKeyFile: cfg.Encryption.KMS.MasterKeyFile,
				MasterKeyHex:  cfg.Encryption.KMS.MasterKeyHex,
			},
		},
	}

	// Use centralized KMS config loading
	masterKeyBytes, err := kmsconfig.LoadMasterKey(kmsCfg)
	if err != nil {
		return "", err
	}

	// Convert to hex string for embedded provider
	masterKeyHex := hex.EncodeToString(masterKeyBytes)
	return masterKeyHex, nil
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
