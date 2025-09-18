package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"progressdb/pkg/kms"
	"progressdb/pkg/security"
)

// setupKMS starts and registers KMS when encryption is enabled.
func (a *App) setupKMS(ctx context.Context) error {
    socket := os.Getenv("PROGRESSDB_KMS_ENDPOINT")
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
		log.Printf("encryption enabled: false")
		return nil
	}

	// master key selection
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

	// Decide KMS mode: embedded (default) or external. Embedded mode uses a
	// local master key (provided in server config) and keeps key material in
	// process memory. External mode assumes an already-running `progressdb-kms` and
	// communicates over the configured socket.
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("PROGRESSDB_KMS_MODE")))
	if mode == "" {
		mode = "embedded"
	}

	switch mode {
	case "embedded":
		// Construct an embedded HashiCorp AEAD provider and register it with
		// the server's security layer so the rest of the code uses the
		// KMS abstraction rather than direct key material handling.
		prov, err := kms.NewHashicorpEmbeddedProvider(ctx, mk)
		if err != nil {
			return fmt.Errorf("failed to initialize embedded KMS provider: %w", err)
		}
		security.RegisterKMSProvider(prov)
		log.Printf("encryption enabled: true (embedded mode, hashicorp AEAD)")
		return nil
	case "external":
		if socket == "" {
			// default to localhost TCP HTTP for external KMS
			socket = "127.0.0.1:6820"
		}
		a.rc = kms.NewRemoteClient(socket)
		security.RegisterKMSProvider(a.rc)
		if err := a.rc.Health(); err != nil {
			return fmt.Errorf("KMS health check failed at %s: %w; ensure KMS is installed and reachable", socket, err)
		}
		kctx, cancel := context.WithCancel(ctx)
		a.cancel = cancel
		go func() { <-kctx.Done() }()
		log.Printf("encryption enabled: true (external KMS socket=%s)", socket)
		return nil
	default:
		return fmt.Errorf("unknown PROGRESSDB_KMS_MODE=%q; must be embedded or external", mode)
	}
}
