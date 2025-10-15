package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"progressdb/pkg/kms"
)

// Registers and starts KMS if encryption is enabled.
func (a *App) setupKMS(ctx context.Context) error {
	kms_endpoint := os.Getenv("PROGRESSDB_KMS_ENDPOINT")
	// dataDir is unused in embedded/external modes; kept for legacy configs.
	useEnc := a.eff.Config.Security.Encryption.Use
	if ev := strings.TrimSpace(os.Getenv("PROGRESSDB_USE_ENCRYPTION")); ev != "" {
		switch strings.ToLower(ev) {
		case "1", "true", "yes":
			useEnc = true
		default:
			useEnc = false
		}
	}

	if !useEnc {
		log.Printf("kms: encryption disabled")
		return nil
	}

	// Select master key from file or hex config.
	var mk string
	switch {
	case strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyFile) != "":
		mkFile := strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyFile)
		keyb, err := os.ReadFile(mkFile)
		if err != nil {
			return fmt.Errorf("failed to read master key file %s: %w", mkFile, err)
		}
		mk = strings.TrimSpace(string(keyb))
	case strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyHex) != "":
		mk = strings.TrimSpace(a.eff.Config.Security.KMS.MasterKeyHex)
	default:
		return fmt.Errorf("PROGRESSDB_USE_ENCRYPTION=true but no master key provided in server config. Set security.kms.master_key_file or security.kms.master_key_hex")
	}
	if mk == "" {
		return fmt.Errorf("master key is empty")
	}
	if kb, err := hex.DecodeString(mk); err != nil || len(kb) != 32 {
		return fmt.Errorf("invalid master_key_hex: must be 64-hex (32 bytes)")
	}

	// Determine mode: embedded (default) or external.
	// Embedded: local master key in process memory.
	// External: connects to `progressdb-kms` at endpoint.
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("PROGRESSDB_KMS_MODE")))
	if mode == "" {
		mode = "embedded"
	}

	switch mode {
	case "embedded":
		if err := kms.RegisterHashicorpEmbeddedProvider(ctx, mk); err != nil {
			return fmt.Errorf("failed to initialize embedded KMS provider: %w", err)
		}
		log.Printf("encryption enabled: true (embedded mode, hashicorp AEAD)")
		return nil
	case "external":
		if kms_endpoint == "" {
			// Use localhost if not specified
			kms_endpoint = "127.0.0.1:6820"
		}
		a.rc = kms.NewRemoteClient(kms_endpoint)
		kms.RegisterKMSProvider(a.rc)
		if err := a.rc.Health(); err != nil {
			return fmt.Errorf("KMS health check failed at %s: %w; ensure KMS is installed and reachable", kms_endpoint, err)
		}
		kctx, cancel := context.WithCancel(ctx)
		a.cancel = cancel
		go func() { <-kctx.Done() }()
		log.Printf("encryption enabled: true (external KMS endpoint=%s)", kms_endpoint)
		return nil
	default:
		return fmt.Errorf("unknown PROGRESSDB_KMS_MODE=%q; must be embedded or external", mode)
	}
}
