package config

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// RuntimeConfig holds derived runtime values that other packages may query
// at runtime (populated during startup by main after merging env+file).
type RuntimeConfig struct {
	BackendKeys map[string]struct{}
	SigningKeys map[string]struct{}
}

var (
	runtimeMu  sync.RWMutex
	runtimeCfg *RuntimeConfig
)

// SetRuntime sets the canonical runtime config used by the running server.
func SetRuntime(rc *RuntimeConfig) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	runtimeCfg = rc
}

// GetBackendKeys returns a copy of configured backend keys.
func GetBackendKeys() map[string]struct{} {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	out := map[string]struct{}{}
	if runtimeCfg == nil || runtimeCfg.BackendKeys == nil {
		return out
	}
	for k := range runtimeCfg.BackendKeys {
		out[k] = struct{}{}
	}
	return out
}

// GetSigningKeys returns a copy of configured signing keys.
func GetSigningKeys() map[string]struct{} {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	out := map[string]struct{}{}
	if runtimeCfg == nil || runtimeCfg.SigningKeys == nil {
		return out
	}
	for k := range runtimeCfg.SigningKeys {
		out[k] = struct{}{}
	}
	return out
}

type Config struct {
	Server struct {
		Address string `yaml:"address"`
		Port    int    `yaml:"port"`
		DBPath  string `yaml:"db_path"`
		TLS     struct {
			CertFile string `yaml:"cert_file"`
			KeyFile  string `yaml:"key_file"`
		} `yaml:"tls"`
	} `yaml:"server"`
	Storage struct {
		DBPath string `yaml:"db_path"`
	} `yaml:"storage"`
	Security struct {
		// EncryptionKey removed - deployments must use an external KMS
		Fields []struct {
			Path      string `yaml:"path"`
			Algorithm string `yaml:"algorithm"`
		} `yaml:"fields"`
		CORS struct {
			AllowedOrigins []string `yaml:"allowed_origins"`
		} `yaml:"cors"`
		RateLimit struct {
			RPS   float64 `yaml:"rps"`
			Burst int     `yaml:"burst"`
		} `yaml:"rate_limit"`
		IPWhitelist []string `yaml:"ip_whitelist"`
		APIKeys     struct {
			Backend  []string `yaml:"backend"`
			Frontend []string `yaml:"frontend"`
			Admin    []string `yaml:"admin"`
		} `yaml:"api_keys"`

		KMS struct {
			Socket        string `yaml:"socket"`
			DataDir       string `yaml:"data_dir"`
			Binary        string `yaml:"binary"`
			MasterKeyFile string `yaml:"master_key_file"`
			// MasterKeyHex may contain the 64-hex KEK directly in the server
			// config. When present the server will embed this value into the
			// child KMS config instead of reading it from a file.
			MasterKeyHex string `yaml:"master_key_hex"`
		} `yaml:"kms"`

		// Encryption block lives under security and controls whether
		// encryption features are active and which fields are protected.
		Encryption struct {
			Use    bool `yaml:"use"`
			Fields []struct {
				Path      string `yaml:"path"`
				Algorithm string `yaml:"algorithm"`
			} `yaml:"fields"`
		} `yaml:"encryption"`
	} `yaml:"security"`
	Logging struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"` // text|json
		HTTP   struct {
			Enabled bool   `yaml:"enabled"`
			URL     string `yaml:"url"`
			Bearer  string `yaml:"bearer"`
		} `yaml:"http"`
	} `yaml:"logging"`
	Validation struct {
		Required []string `yaml:"required"`
		Types    []struct {
			Path string `yaml:"path"`
			Type string `yaml:"type"` // string|number|boolean|object|array
		} `yaml:"types"`
		MaxLen []struct {
			Path string `yaml:"path"`
			Max  int    `yaml:"max"`
		} `yaml:"max_len"`
		Enums []struct {
			Path   string   `yaml:"path"`
			Values []string `yaml:"values"`
		} `yaml:"enums"`
		WhenThen []struct {
			When struct {
				Path   string      `yaml:"path"`
				Equals interface{} `yaml:"equals"`
			} `yaml:"when"`
			Then struct {
				Required []string `yaml:"required"`
			} `yaml:"then"`
		} `yaml:"when_then"`
	} `yaml:"validation"`
}

// Addr returns host:port for HTTP server.
func (c *Config) Addr() string {
	addr := c.Server.Address
	if addr == "" {
		addr = "0.0.0.0"
	}
	p := c.Server.Port
	if p == 0 {
		p = 8080
	}
	return fmt.Sprintf("%s:%d", addr, p)
}

func Load(path string) (*Config, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ParseCommandFlags defines and parses command-line flags and returns their
// values along with a map indicating which flags were explicitly set.
func ParseCommandFlags() (addr string, dbPath string, cfgPath string, setFlags map[string]bool) {
	addrPtr := flag.String("addr", ":8080", "HTTP listen address")
	dbPtr := flag.String("db", "./.database", "Pebble DB path")
	cfgPtr := flag.String("config", "./config.yaml", "Path to config file")
	flag.Parse()
	setFlags = map[string]bool{}
	flag.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })
	return *addrPtr, *dbPtr, *cfgPtr, setFlags
}

// LoadEnvOverrides applies environment overrides onto the provided cfg and
// returns derived backend and signing key maps plus whether env vars were used.
func LoadEnvOverrides(cfg *Config) (map[string]struct{}, map[string]struct{}, bool) {
	envUsed := false
	parseList := func(v string) []string {
		if v == "" {
			return nil
		}
		parts := []string{}
		for _, p := range strings.Split(v, ",") {
			if s := strings.TrimSpace(p); s != "" {
				parts = append(parts, s)
			}
		}
		return parts
	}

	// Server-scoped env vars take precedence when present.
	if v := os.Getenv("PROGRESSDB_SERVER_ADDR"); v != "" {
		envUsed = true
		if h, p, err := net.SplitHostPort(v); err == nil {
			cfg.Server.Address = h
			if pi, err := strconv.Atoi(p); err == nil {
				cfg.Server.Port = pi
			}
		} else {
			cfg.Server.Address = v
		}
	} else if v := os.Getenv("PROGRESSDB_ADDR"); v != "" {
		envUsed = true
		if h, p, err := net.SplitHostPort(v); err == nil {
			cfg.Server.Address = h
			if pi, err := strconv.Atoi(p); err == nil {
				cfg.Server.Port = pi
			}
		} else {
			cfg.Server.Address = v
		}
	} else {
		if host := os.Getenv("PROGRESSDB_SERVER_ADDRESS"); host != "" {
			envUsed = true
			cfg.Server.Address = host
		}
		if port := os.Getenv("PROGRESSDB_SERVER_PORT"); port != "" {
			envUsed = true
			if pi, err := strconv.Atoi(port); err == nil {
				cfg.Server.Port = pi
			}
		}
	}

    if v := os.Getenv("PROGRESSDB_SERVER_DB_PATH"); v != "" {
        envUsed = true
        cfg.Server.DBPath = v
    } else if v := os.Getenv("PROGRESSDB_DB_PATH"); v != "" {
        envUsed = true
        cfg.Server.DBPath = v
    }
	// Encryption fields: comma-separated list of field paths to encrypt.
	// Use PROGRESSDB_ENCRYPTION_FIELDS to populate the config encryption
	// block for dev convenience (overrides config file when present).
	if v := os.Getenv("PROGRESSDB_ENCRYPTION_FIELDS"); v != "" {
		envUsed = true
		parts := parseList(v)
		// Populate security.encryption.fields (preferred) and set
		// security.fields for compatibility with older code.
		cfg.Security.Fields = nil
		cfg.Security.Encryption.Fields = nil
		for _, p := range parts {
			entry := struct {
				Path      string `yaml:"path"`
				Algorithm string `yaml:"algorithm"`
			}{Path: p, Algorithm: "aes-gcm"}
			cfg.Security.Fields = append(cfg.Security.Fields, entry)
			cfg.Security.Encryption.Fields = append(cfg.Security.Encryption.Fields, entry)
		}
	}
	if v := os.Getenv("PROGRESSDB_CORS_ORIGINS"); v != "" {
		envUsed = true
		cfg.Security.CORS.AllowedOrigins = parseList(v)
	}
	if v := os.Getenv("PROGRESSDB_RATE_RPS"); v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			envUsed = true
			cfg.Security.RateLimit.RPS = f
		}
	}
	if v := os.Getenv("PROGRESSDB_RATE_BURST"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			envUsed = true
			cfg.Security.RateLimit.Burst = n
		}
	}
	if v := os.Getenv("PROGRESSDB_IP_WHITELIST"); v != "" {
		envUsed = true
		cfg.Security.IPWhitelist = parseList(v)
	}
	if v := os.Getenv("PROGRESSDB_API_BACKEND_KEYS"); v != "" {
		envUsed = true
		cfg.Security.APIKeys.Backend = parseList(v)
	}
	if v := os.Getenv("PROGRESSDB_API_FRONTEND_KEYS"); v != "" {
		envUsed = true
		cfg.Security.APIKeys.Frontend = parseList(v)
	}
	if v := os.Getenv("PROGRESSDB_API_ADMIN_KEYS"); v != "" {
		envUsed = true
		cfg.Security.APIKeys.Admin = parseList(v)
	}
	// The option to allow unauthenticated API access has been removed;
	// API access requires a valid API key. Any legacy env var is ignored.

	// KMS related env overrides
	if v := os.Getenv("PROGRESSDB_KMS_SOCKET"); v != "" {
		envUsed = true
		cfg.Security.KMS.Socket = v
	}
	if v := os.Getenv("PROGRESSDB_KMS_DATA_DIR"); v != "" {
		envUsed = true
		cfg.Security.KMS.DataDir = v
	}
	if v := os.Getenv("PROGRESSDB_KMS_BINARY"); v != "" {
		envUsed = true
		cfg.Security.KMS.Binary = v
	}
	// Allow optional provisioning of the master key via env for
	// convenience in dev scenarios. File-based master keys are still
	// preferred and take precedence when set in the server config.
	if v := os.Getenv("PROGRESSDB_KMS_MASTER_KEY_FILE"); v != "" {
		envUsed = true
		cfg.Security.KMS.MasterKeyFile = v
	}
	if v := os.Getenv("PROGRESSDB_KMS_MASTER_KEY_HEX"); v != "" {
		envUsed = true
		cfg.Security.KMS.MasterKeyHex = v
	}
	// the security.encryption.use flag which controls whether encryption
	// features are active; KMS configuration is separate.
	if v := os.Getenv("PROGRESSDB_USE_ENCRYPTION"); v != "" {
		envUsed = true
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes":
			cfg.Security.Encryption.Use = true
		default:
			cfg.Security.Encryption.Use = false
		}
	}
	// The KMS master key may be provided via server config (preferred) or
	// via environment variables for convenience in development. The
	// precedence for the effective master key is: config.master_key_file
	// (file) -> config.master_key_hex (embedded) -> env-provided hex.
	// Authorization and production-grade provisioning should use files
	// or orchestrator secrets rather than environment variables.
	if c := os.Getenv("PROGRESSDB_TLS_CERT"); c != "" {
		envUsed = true
		cfg.Server.TLS.CertFile = c
	}
	if k := os.Getenv("PROGRESSDB_TLS_KEY"); k != "" {
		envUsed = true
		cfg.Server.TLS.KeyFile = k
	}

	backendKeys := map[string]struct{}{}
	for _, k := range cfg.Security.APIKeys.Backend {
		backendKeys[k] = struct{}{}
	}
	signingKeys := map[string]struct{}{}
	for k := range backendKeys {
		signingKeys[k] = struct{}{}
	}
	// Signing keys are identical to backend API keys (no separate fallback).
	return backendKeys, signingKeys, envUsed
}

// LoadEffective loads config from the given path (file) and applies environment
// overrides. It returns the effective config, runtime key maps and a boolean
// indicating whether env vars were used.
func LoadEffective(path string) (*Config, map[string]struct{}, map[string]struct{}, bool, error) {
	cfg, err := Load(path)
	if err != nil {
		cfg = &Config{}
	}
	backendKeys, signingKeys, envUsed := LoadEnvOverrides(cfg)
	return cfg, backendKeys, signingKeys, envUsed, nil
}

// (no key-id helpers; middleware will try all keys)

// ResolveConfigPath decides the config file path using the flag-provided value
// and the environment variable `PROGRESSDB_CONFIG` when the flag was not set.
func ResolveConfigPath(flagPath string, flagSet bool) string {
	if flagSet {
		return flagPath
	}
	// prefer server-scoped env var
	if p := os.Getenv("PROGRESSDB_SERVER_CONFIG"); p != "" {
		return p
	}
	if p := os.Getenv("PROGRESSDB_CONFIG"); p != "" {
		return p
	}
	return flagPath
}
