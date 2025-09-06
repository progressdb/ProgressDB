package config

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"

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
		TLS     struct {
			CertFile string `yaml:"cert_file"`
			KeyFile  string `yaml:"key_file"`
		} `yaml:"tls"`
	} `yaml:"server"`
	Storage struct {
		DBPath string `yaml:"db_path"`
	} `yaml:"storage"`
	Security struct {
		EncryptionKey string `yaml:"encryption_key"`
		Fields        []struct {
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
			Backend     []string `yaml:"backend"`
			Frontend    []string `yaml:"frontend"`
			Admin       []string `yaml:"admin"`
			AllowUnauth bool     `yaml:"allow_unauth"`
		} `yaml:"api_keys"`
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

// LoadEffective loads config from the given path (if present), then applies
// environment variable overrides. It returns the effective Config and a small
// AppConfig object containing canonical runtime values such as backend and
// signing keys.
func LoadEffective(path string) (*Config, map[string]struct{}, map[string]struct{}, bool, error) {
	cfg, err := Load(path)
	if err != nil {
		// If no config file, continue with empty defaults.
		cfg = &Config{}
	}
	envUsed := false

	// Helper to parse comma-separated env var into slice
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

	// Addr override (ADDRESS or ADDRESS+PORT)
	if v := os.Getenv("PROGRESSDB_ADDR"); v != "" {
		// attempt to split host:port if provided
		envUsed = true
		if h, p, err := net.SplitHostPort(v); err == nil {
			cfg.Server.Address = h
			if pi, err := strconv.Atoi(p); err == nil {
				cfg.Server.Port = pi
			}
		} else {
			// no port included, set as address
			cfg.Server.Address = v
		}
	} else {
		if host := os.Getenv("PROGRESSDB_ADDRESS"); host != "" {
			envUsed = true
			cfg.Server.Address = host
		}
		if port := os.Getenv("PROGRESSDB_PORT"); port != "" {
			envUsed = true
			if pi, err := strconv.Atoi(port); err == nil {
				cfg.Server.Port = pi
			}
		}
	}

	// Storage
	if v := os.Getenv("PROGRESSDB_DB_PATH"); v != "" {
		envUsed = true
		cfg.Storage.DBPath = v
	}

	// Security: encryption key
	if v := os.Getenv("PROGRESSDB_ENCRYPTION_KEY"); v != "" {
		envUsed = true
		cfg.Security.EncryptionKey = v
	}
	// Security: encrypt fields
	if v := os.Getenv("PROGRESSDB_ENCRYPT_FIELDS"); v != "" {
		parts := parseList(v)
		envUsed = true
		cfg.Security.Fields = nil
		for _, p := range parts {
			cfg.Security.Fields = append(cfg.Security.Fields, struct {
				Path      string `yaml:"path"`
				Algorithm string `yaml:"algorithm"`
			}{Path: p, Algorithm: "aes-gcm"})
		}
	}

	// CORS
	if v := os.Getenv("PROGRESSDB_CORS_ORIGINS"); v != "" {
		envUsed = true
		cfg.Security.CORS.AllowedOrigins = parseList(v)
	}

	// Rate limits
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

	// IP whitelist
	if v := os.Getenv("PROGRESSDB_IP_WHITELIST"); v != "" {
		envUsed = true
		cfg.Security.IPWhitelist = parseList(v)
	}

	// API keys
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
	if v := os.Getenv("PROGRESSDB_ALLOW_UNAUTH"); v != "" {
		envUsed = true
		cfg.Security.APIKeys.AllowUnauth = strings.ToLower(v) == "true" || v == "1"
	}

	// TLS cert/key from env (compat)
	if c := os.Getenv("PROGRESSDB_TLS_CERT"); c != "" {
		envUsed = true
		cfg.Server.TLS.CertFile = c
	}
	if k := os.Getenv("PROGRESSDB_TLS_KEY"); k != "" {
		envUsed = true
		cfg.Server.TLS.KeyFile = k
	}

	// Build maps for backend keys and signing keys
	backendKeys := map[string]struct{}{}
	for _, k := range cfg.Security.APIKeys.Backend {
		backendKeys[k] = struct{}{}
	}

	signingKeys := map[string]struct{}{}
	// by default signing keys == backend keys
	for k := range backendKeys {
		signingKeys[k] = struct{}{}
	}
	// include explicit compat env var
	if s := os.Getenv("AUTHOR_SIGNING_SECRET"); s != "" {
		envUsed = true
		signingKeys[s] = struct{}{}
	}

	return cfg, backendKeys, signingKeys, envUsed, nil
}

// ResolveConfigPath decides the config file path using the flag-provided value
// and the environment variable `PROGRESSDB_CONFIG` when the flag was not set.
func ResolveConfigPath(flagPath string, flagSet bool) string {
	if flagSet {
		return flagPath
	}
	if p := os.Getenv("PROGRESSDB_CONFIG"); p != "" {
		return p
	}
	return flagPath
}
