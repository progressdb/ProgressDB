package app

import (
	"encoding/hex"
	"fmt"
	"os"

	"progressdb/pkg/config"
)

// validateConfig performs quick, fail-fast validation of the effective
// configuration before starting long-running services. Keep checks light
// and focused so callers can surface user-friendly errors.
func validateConfig(eff config.EffectiveConfigResult) error {
	// DB path must be present
	if p := eff.DBPath; p == "" {
		return fmt.Errorf("database path is empty: set --db flag, PROGRESSDB_DB_PATH env, or server.db_path in config")
	}

	// TLS cert/key presence check if one is set
	cert := eff.Config.Server.TLS.CertFile
	key := eff.Config.Server.TLS.KeyFile
	if (cert != "" && key == "") || (cert == "" && key != "") {
		return fmt.Errorf("incomplete TLS configuration: both server.tls.cert_file and server.tls.key_file must be set")
	}
	if cert != "" {
		if _, err := os.Stat(cert); err != nil {
			return fmt.Errorf("tls cert file not accessible: %w", err)
		}
		if _, err := os.Stat(key); err != nil {
			return fmt.Errorf("tls key file not accessible: %w", err)
		}
	}

	// If encryption is enabled (either in config or via env), ensure a master key is provided
	useEnc := eff.Config.Security.Encryption.Use
	if ev := os.Getenv("PROGRESSDB_USE_ENCRYPTION"); ev != "" {
		switch ev := ev; ev {
		case "1", "true", "yes", "True", "TRUE":
			useEnc = true
		default:
			useEnc = false
		}
	}
	if useEnc {
		mkFile := eff.Config.Security.KMS.MasterKeyFile
		mkHex := eff.Config.Security.KMS.MasterKeyHex
		if mkFile == "" && mkHex == "" {
			return fmt.Errorf("encryption enabled but no master key provided: set security.kms.master_key_file or security.kms.master_key_hex")
		}
		if mkHex != "" {
			if _, err := hex.DecodeString(mkHex); err != nil {
				return fmt.Errorf("invalid master_key_hex: %w", err)
			}
		}
	}

	return nil
}
