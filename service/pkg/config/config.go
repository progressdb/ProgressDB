package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/adhocore/gronx"
	"github.com/goccy/go-yaml"
)

var (
	runtimeMu  sync.RWMutex
	runtimeCfg *RuntimeConfig
	fullConfig *Config
)

func SetRuntime(rc *RuntimeConfig) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	runtimeCfg = rc
}

func SetConfig(full *Config) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	fullConfig = full
}

func GetConfig() *Config {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	return fullConfig
}

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

func GetMaxPayloadSize() int {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	if runtimeCfg == nil || runtimeCfg.MaxPayloadSize == 0 {
		return 102400
	}
	return int(runtimeCfg.MaxPayloadSize)
}

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

func (c *Config) ValidateConfigCron() error {
	if !gronx.IsValid(c.Retention.Cron) {
		return fmt.Errorf("invalid retention cron expression: %s", c.Retention.Cron)
	}
	return nil
}

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
