package app

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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
	useEnc := eff.Config.Encryption.Enabled
	if ev := os.Getenv("PROGRESSDB_ENCRYPTION_ENABLED"); ev != "" {
		switch ev := ev; ev {
		case "1", "true", "yes", "True", "TRUE":
			useEnc = true
		default:
			useEnc = false
		}
	}
	if useEnc {
		mkFile := eff.Config.Encryption.KMS.MasterKeyFile
		mkHex := eff.Config.Encryption.KMS.MasterKeyHex
		if mkFile == "" && mkHex == "" {
			return fmt.Errorf("encryption enabled but no master key provided: set security.kms.master_key_file or security.kms.master_key_hex")
		}
		if mkHex != "" {
			if _, err := hex.DecodeString(mkHex); err != nil {
				return fmt.Errorf("invalid master_key_hex: %w", err)
			}
		}
	}

	// Retention validation: if retention configured, validate durations and cron-ish syntax.
	ret := eff.Config.Retention
	// if retention isn't explicitly configured, nothing to validate
	if ret != (config.RetentionConfig{}) {
		// set sensible defaults if empty
		if ret.MinPeriod == "" {
			ret.MinPeriod = "1h"
		}
		// parse durations
		parseDur := func(s string) (time.Duration, error) {
			s = strings.TrimSpace(s)
			if s == "" {
				return 0, fmt.Errorf("empty duration")
			}
			// support days like "30d"
			if strings.HasSuffix(s, "d") {
				v := strings.TrimSuffix(s, "d")
				n, err := strconv.Atoi(v)
				if err != nil {
					return 0, err
				}
				return time.Duration(n) * 24 * time.Hour, nil
			}
			// fallback to time.ParseDuration
			return time.ParseDuration(s)
		}
		minD, err := parseDur(ret.MinPeriod)
		if err != nil {
			return fmt.Errorf("invalid retention.min_period: %w", err)
		}
		if ret.Period != "" {
			pd, err := parseDur(ret.Period)
			if err != nil {
				return fmt.Errorf("invalid retention.period: %w", err)
			}
			if pd < minD {
				return fmt.Errorf("retention.period %s is less than minimum allowed %s", ret.Period, ret.MinPeriod)
			}
		}
		// quick cron-ish validation: 5 or 6 space-separated fields
		if ret.Cron != "" {
			parts := strings.Fields(ret.Cron)
			if len(parts) < 5 || len(parts) > 6 {
				return fmt.Errorf("invalid retention.cron: must be 5-6 space-separated cron fields")
			}
		}
	}

	return nil
}
