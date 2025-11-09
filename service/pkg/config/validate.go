package config

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/adhocore/gronx"
)

// set defaults, fail fast on critical errors
func ValidateConfig(eff EffectiveConfigResult) error {
	cfg := eff.Config
	if cfg == nil {
		return fmt.Errorf("effective config is nil")
	}
	// DB path must be present
	if p := eff.DBPath; p == "" {
		return fmt.Errorf("database path is empty: set --db flag, PROGRESSDB_DB_PATH env, or server.db_path in config")
	}

	// Signing keys are required
	if len(cfg.Server.APIKeys.Signing) == 0 {
		return fmt.Errorf("signing keys are required: set server.api_keys.signing in config or PROGRESSDB_API_SIGNING_KEYS env")
	}

	// If encryption is enabled (either in config or via env), ensure a master key is provided
	useEnc := cfg.Encryption.Enabled
	if ev := os.Getenv("PROGRESSDB_ENCRYPTION_ENABLED"); ev != "" {
		switch ev := ev; ev {
		case "1", "true", "yes", "True", "TRUE":
			useEnc = true
		default:
			useEnc = false
		}
	}
	if useEnc {
		mkFile := cfg.Encryption.KMS.MasterKeyFile
		mkHex := cfg.Encryption.KMS.MasterKeyHex
		if mkFile == "" && mkHex == "" {
			return fmt.Errorf("encryption enabled but no master key provided: set security.kms.master_key_file or security.kms.master_key_hex")
		}
		if mkHex != "" {
			if _, err := hex.DecodeString(mkHex); err != nil {
				return fmt.Errorf("invalid master_key_hex: %w", err)
			}
		}
	}

	// Retention validation: if retention is enabled, validate durations and cron-ish syntax.
	ret := cfg.Retention
	if ret.Enabled {
		if ret.Cron != "" {
			gron := gronx.New()
			if !gron.IsValid(ret.Cron) {
				return fmt.Errorf("invalid retention.cron: not a valid cron expression")
			}
		}
		if ret.MTTL <= 0 {
			return fmt.Errorf("invalid retention.mttl: must be positive duration")
		}
		if ret.TTTL <= 0 {
			return fmt.Errorf("invalid retention.tttl: must be positive duration")
		}
	}

	return nil
}
