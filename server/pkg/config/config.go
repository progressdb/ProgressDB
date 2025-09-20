package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"sync"
)

// RuntimeConfig holds runtime key sets for use by other packages.
type RuntimeConfig struct {
	BackendKeys map[string]struct{}
	SigningKeys map[string]struct{}
}

var (
	runtimeMu  sync.RWMutex
	runtimeCfg *RuntimeConfig
)

// SetRuntime sets the global runtime config.
func SetRuntime(rc *RuntimeConfig) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	runtimeCfg = rc
}

// GetBackendKeys returns a copy of backend API keys.
func GetBackendKeys() map[string]struct{} {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	out := make(map[string]struct{})
	if runtimeCfg == nil || runtimeCfg.BackendKeys == nil {
		return out
	}
	for k := range runtimeCfg.BackendKeys {
		out[k] = struct{}{}
	}
	return out
}

// GetSigningKeys returns a copy of signing keys.
func GetSigningKeys() map[string]struct{} {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	out := make(map[string]struct{})
	if runtimeCfg == nil || runtimeCfg.SigningKeys == nil {
		return out
	}
	for k := range runtimeCfg.SigningKeys {
		out[k] = struct{}{}
	}
	return out
}

// Config is the main configuration struct.
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
	Security struct {
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
		Encryption struct {
			Use    bool     `yaml:"use"`
			Fields []string `yaml:"fields"`
		} `yaml:"encryption"`
		KMS struct {
			Endpoint      string `yaml:"endpoint"`
			DataDir       string `yaml:"data_dir"`
			Binary        string `yaml:"binary"`
			MasterKeyFile string `yaml:"master_key_file"`
			MasterKeyHex  string `yaml:"master_key_hex"`
		} `yaml:"kms"`
	} `yaml:"security"`
	Logging struct {
		Level string `yaml:"level"`
	} `yaml:"logging"`
}

// Addr returns the HTTP server address as host:port.
func (c *Config) Addr() string {
	addr := c.Server.Address
	if addr == "" {
		addr = "0.0.0.0"
	}
	port := c.Server.Port
	if port == 0 {
		port = 8080
	}
	return fmt.Sprintf("%s:%d", addr, port)
}

// Load reads and parses a config file.
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

// ResolveConfigPath returns the config file path, preferring flag, then env.
func ResolveConfigPath(flagPath string, flagSet bool) string {
	if flagSet {
		return flagPath
	}
	if p := os.Getenv("PROGRESSDB_SERVER_CONFIG"); p != "" {
		return p
	}
	if p := os.Getenv("PROGRESSDB_CONFIG"); p != "" {
		return p
	}
	return flagPath
}
