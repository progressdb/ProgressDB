package encryption

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"progressdb/pkg/state/logger"

	"github.com/progressdb/kms/pkg/kms"
	"github.com/progressdb/kms/pkg/store"
	"progressdb/pkg/config"
	"progressdb/pkg/state"
)

var (
	embeddedKMS  *kms.KMS
	remoteClient *RemoteClient
	useEmbedded  bool
)

func SetupKMS(ctx context.Context) error {
	cfg := config.GetConfig()

	if !cfg.Encryption.Enabled {
		logger.Info("kms: encryption disabled")
		return nil
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.Encryption.KMS.Mode))
	if mode == "" {
		mode = "embedded"
	}

	switch mode {
	case "embedded":
		return setupEmbeddedKMS(ctx, cfg)
	case "external":
		return setupExternalKMS(cfg.Encryption.KMS.Endpoint)
	default:
		return fmt.Errorf("unknown KMS mode=%q; must be embedded or external", mode)
	}
}

func setupEmbeddedKMS(ctx context.Context, cfg *config.Config) error {
	masterKeyHex, err := getMasterKey(cfg)
	if err != nil {
		return err
	}

	// Use standardized KMS path from state package
	dbPath := state.KMSPath(cfg.Server.DBPath)

	// Clean any whitespace
	masterKeyHex = strings.TrimSpace(masterKeyHex)

	masterKeyBytes, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return fmt.Errorf("failed to decode master key hex: %w", err)
	}

	// Ensure key is exactly 32 bytes for AES-256-GCM
	if len(masterKeyBytes) != 32 {
		// Hash the key to get exactly 32 bytes
		hash := sha256.Sum256(masterKeyBytes)
		masterKeyBytes = hash[:]
		logger.Info("kms: master key hashed to 32 bytes for AES-256-GCM")
	}

	st, err := store.New(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create KMS store: %w", err)
	}

	embeddedKMS, err = kms.New(ctx, st, masterKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to initialize embedded KMS: %w", err)
	}

	useEmbedded = true
	logger.Info("encryption enabled: true (embedded mode)", "db_path", dbPath)
	return nil
}

func setupExternalKMS(kmsEndpoint string) error {
	remoteClient = NewRemoteClient(kmsEndpoint)
	useEmbedded = false

	if err := remoteClient.Health(); err != nil {
		return fmt.Errorf("KMS health check failed at %s: %w; ensure KMS is installed and reachable", kmsEndpoint, err)
	}

	logger.Info("encryption enabled: true (external KMS)", "endpoint", kmsEndpoint)
	return nil
}

func getMasterKey(cfg *config.Config) (string, error) {
	if cfg.Encryption.KMS.MasterKeyHex != "" {
		return cfg.Encryption.KMS.MasterKeyHex, nil
	}

	if cfg.Encryption.KMS.MasterKeyFile != "" {
		data, err := os.ReadFile(cfg.Encryption.KMS.MasterKeyFile)
		if err != nil {
			return "", fmt.Errorf("reading master key file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	return "", fmt.Errorf("no master key configured: set either master_key_file or master_key_hex")
}

func IsProviderEnabled() bool {
	return embeddedKMS != nil || remoteClient != nil
}

func CreateDEK(keyID ...string) (string, []byte, string, string, error) {
	if useEmbedded && embeddedKMS != nil {
		dek, err := embeddedKMS.CreateDEK(keyID...)
		if err != nil {
			return "", nil, "", "", err
		}
		return dek.KeyID, dek.WrappedDEK, dek.KekID, dek.KekVersion, nil
	} else if !useEmbedded && remoteClient != nil {
		return remoteClient.CreateDEK(keyID...)
	}
	return "", nil, "", "", fmt.Errorf("no KMS initialized")
}

func EncryptWithDEK(keyID string, plaintext, aad []byte) ([]byte, string, error) {
	if useEmbedded && embeddedKMS != nil {
		ciphertext, err := embeddedKMS.Encrypt(keyID, plaintext)
		if err != nil {
			return nil, "", err
		}
		return ciphertext, "", nil
	} else if !useEmbedded && remoteClient != nil {
		return remoteClient.EncryptWithDEK(keyID, plaintext, aad)
	}
	return nil, "", fmt.Errorf("no KMS initialized")
}

func DecryptWithDEK(keyID string, ciphertext, aad []byte) ([]byte, error) {
	if useEmbedded && embeddedKMS != nil {
		return embeddedKMS.Decrypt(keyID, ciphertext)
	} else if !useEmbedded && remoteClient != nil {
		return remoteClient.DecryptWithDEK(keyID, ciphertext, aad)
	}
	return nil, fmt.Errorf("no KMS initialized")
}
