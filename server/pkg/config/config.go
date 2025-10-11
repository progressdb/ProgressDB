package config

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Defaults and limits for queue/WAL configuration
const (
	DefaultQueueCapacity    = 64 * 1024
	DefaultWALMaxFileSize   = 64 * 1024 * 1024 // 64 MiB
	DefaultWALBatchInterval = 100 * time.Millisecond
	DefaultWALBatchSize     = 100
	MinWALFileSize          = 1 * 1024 * 1024 // 1 MiB
	MinWALBatchInterval     = 10 * time.Millisecond
	DefaultCompressMinBytes = 512
)

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

// LoadConfigFile reads and parses a config file.
func LoadConfigFile(path string) (*Config, error) {
	b, err := os.ReadFile(path)
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

// Validate applies defaults and validates values in the config. It mutates
// the receiver to fill in missing defaults and returns an error if any
// configuration value is invalid.
func (c *Config) ValidateConfig() error {
	// Queue defaults
	if c.Ingest.Queue.Capacity <= 0 {
		c.Ingest.Queue.Capacity = DefaultQueueCapacity
	}

	// WAL defaults and validation
	wc := &c.Ingest.Queue.WAL
	if wc.MaxFileSize.Int64() == 0 {
		wc.MaxFileSize = SizeBytes(DefaultWALMaxFileSize)
	}
	if wc.MaxFileSize.Int64() < MinWALFileSize {
		return fmt.Errorf("wal.max_file_size must be >= %d bytes", MinWALFileSize)
	}
	if wc.BatchInterval.Duration() == 0 {
		wc.BatchInterval = Duration(DefaultWALBatchInterval)
	}
	if wc.BatchInterval.Duration() < MinWALBatchInterval {
		return fmt.Errorf("wal.batch_interval must be >= %s", MinWALBatchInterval)
	}
	if wc.BatchSize <= 0 {
		wc.BatchSize = DefaultWALBatchSize
	}
	if wc.CompressMinBytes == 0 {
		wc.CompressMinBytes = DefaultCompressMinBytes
	}

	// Normalize mode
	switch wc.Mode {
	case "", "batch", "sync", "none":
		// ok
	default:
		return fmt.Errorf("wal.mode must be one of: none, batch, sync")
	}
	if !wc.Enabled {
		wc.Mode = "none"
	}

	return nil
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

// SetDefaultRetention is kept for compatibility; types declared in types.go.
func (c *Config) SetDefaultRetention() {
	// noop placeholder for backward compatibility
}
