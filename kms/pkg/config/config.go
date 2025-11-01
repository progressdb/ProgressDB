package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	configMu  sync.RWMutex
	globalCfg *Config
	masterKey []byte
)

// Config is the main configuration struct for KMS (compatible with service config)
type Config struct {
	Encryption EncryptionConfig `yaml:"encryption"`
}

// EncryptionConfig holds encryption related settings.
type EncryptionConfig struct {
	Enabled bool     `yaml:"enabled,default=true"`
	Fields  []string `yaml:"fields,default=[\"body.content\"]"`
	KMS     struct {
		Mode          string `yaml:"mode,default=embedded"`
		Endpoint      string `yaml:"endpoint,default=127.0.0.1:6820"`
		DataDir       string `yaml:"data_dir,default=/kms"`
		Binary        string `yaml:"binary,default=/usr/local/bin/progressdb-kms"`
		MasterKeyFile string `yaml:"master_key_file"`
		MasterKeyHex  string `yaml:"master_key_hex"`
	} `yaml:"kms"`
}

// SizeBytes represents a number of bytes, unmarshaled from human-friendly strings like "64MB" or plain integers.
type SizeBytes int64

// UnmarshalYAML implements custom YAML unmarshaling for SizeBytes
func (s *SizeBytes) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		*s = 0
		return nil
	}

	var raw string
	if err := value.Decode(&raw); err != nil {
		return err
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		*s = 0
		return nil
	}

	// Try to parse as human-readable bytes (simple implementation)
	if strings.HasSuffix(raw, "KB") || strings.HasSuffix(raw, "kb") {
		num := strings.TrimSuffix(raw, "KB")
		num = strings.TrimSuffix(num, "kb")
		if i, err := strconv.ParseInt(num, 10, 64); err == nil {
			*s = SizeBytes(i * 1024)
			return nil
		}
	}
	if strings.HasSuffix(raw, "MB") || strings.HasSuffix(raw, "mb") {
		num := strings.TrimSuffix(raw, "MB")
		num = strings.TrimSuffix(num, "mb")
		if i, err := strconv.ParseInt(num, 10, 64); err == nil {
			*s = SizeBytes(i * 1024 * 1024)
			return nil
		}
	}
	if strings.HasSuffix(raw, "GB") || strings.HasSuffix(raw, "gb") {
		num := strings.TrimSuffix(raw, "GB")
		num = strings.TrimSuffix(num, "gb")
		if i, err := strconv.ParseInt(num, 10, 64); err == nil {
			*s = SizeBytes(i * 1024 * 1024 * 1024)
			return nil
		}
	}

	// Try to parse as plain integer
	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		*s = SizeBytes(i)
		return nil
	}

	return fmt.Errorf("invalid size value: %q", raw)
}

func (s SizeBytes) Int64() int64 { return int64(s) }

// Duration is a wrapper around time.Duration that supports YAML parsing from strings like "100ms" or plain numbers (interpreted as seconds).
type Duration time.Duration

// UnmarshalYAML implements custom YAML unmarshaling for Duration
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		*d = Duration(0)
		return nil
	}

	var raw string
	if err := value.Decode(&raw); err != nil {
		return err
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		*d = Duration(0)
		return nil
	}

	// Try to parse as duration
	if td, err := time.ParseDuration(raw); err == nil {
		*d = Duration(td)
		return nil
	}

	// Try to parse as numeric seconds
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		*d = Duration(time.Duration(f * float64(time.Second)))
		return nil
	}

	return fmt.Errorf("invalid duration value: %q", raw)
}

func (d Duration) Duration() time.Duration { return time.Duration(d) }

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	cfg := &Config{}

	// Load from file if provided
	if configPath != "" {
		if err := loadFromFile(configPath, cfg); err != nil {
			return nil, fmt.Errorf("failed to load config from file: %w", err)
		}
	}

	// Override with environment variables
	overrideWithEnv(cfg)

	// Set defaults for any remaining empty values
	setDefaults(cfg)

	return cfg, nil
}

// loadFromFile loads configuration from a YAML file
func loadFromFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File not found is OK, we'll use defaults
		}
		return fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}

	return nil
}

// overrideWithEnv overrides config values with environment variables
func overrideWithEnv(cfg *Config) {
	// Support service environment variables
	if enabled := os.Getenv("PROGRESSDB_ENCRYPTION_ENABLED"); enabled != "" {
		cfg.Encryption.Enabled = enabled == "true"
	}
	if fields := os.Getenv("PROGRESSDB_ENCRYPTION_FIELDS"); fields != "" {
		cfg.Encryption.Fields = strings.Split(fields, ",")
	}

	// KMS-specific environment variables
	if mode := os.Getenv("PROGRESSDB_KMS_MODE"); mode != "" {
		cfg.Encryption.KMS.Mode = mode
	}
	if endpoint := os.Getenv("PROGRESSDB_KMS_ENDPOINT"); endpoint != "" {
		cfg.Encryption.KMS.Endpoint = endpoint
	}
	if dataDir := os.Getenv("PROGRESSDB_KMS_DATA_DIR"); dataDir != "" {
		cfg.Encryption.KMS.DataDir = dataDir
	}
	if binary := os.Getenv("PROGRESSDB_KMS_BINARY"); binary != "" {
		cfg.Encryption.KMS.Binary = binary
	}
	if masterKeyFile := os.Getenv("PROGRESSDB_KMS_MASTER_KEY_FILE"); masterKeyFile != "" {
		cfg.Encryption.KMS.MasterKeyFile = masterKeyFile
	}
	if masterKeyHex := os.Getenv("PROGRESSDB_KMS_MASTER_KEY_HEX"); masterKeyHex != "" {
		cfg.Encryption.KMS.MasterKeyHex = masterKeyHex
	}
}

// setDefaults sets default values for empty configuration fields
func setDefaults(cfg *Config) {
	// Set encryption defaults
	if !cfg.Encryption.Enabled {
		cfg.Encryption.Enabled = true
	}
	if len(cfg.Encryption.Fields) == 0 {
		cfg.Encryption.Fields = []string{"body.content"}
	}

	// Set KMS defaults
	if cfg.Encryption.KMS.Mode == "" {
		cfg.Encryption.KMS.Mode = "embedded"
	}
	if cfg.Encryption.KMS.Endpoint == "" {
		cfg.Encryption.KMS.Endpoint = "127.0.0.1:6820"
	}
	if cfg.Encryption.KMS.DataDir == "" {
		cfg.Encryption.KMS.DataDir = "/kms"
	}
	if cfg.Encryption.KMS.Binary == "" {
		cfg.Encryption.KMS.Binary = "/usr/local/bin/progressdb-kms"
	}
}

// SetGlobalConfig stores the configuration globally
func SetGlobalConfig(cfg *Config) {
	configMu.Lock()
	defer configMu.Unlock()
	globalCfg = cfg
}

// GetGlobalConfig returns the global configuration
func GetGlobalConfig() *Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return globalCfg
}

// SetMasterKey stores the master key globally
func SetMasterKey(key []byte) {
	configMu.Lock()
	defer configMu.Unlock()
	masterKey = key
}

// GetMasterKey returns the global master key
func GetMasterKey() []byte {
	configMu.RLock()
	defer configMu.RUnlock()
	return masterKey
}

// GetKMSConfig returns just the KMS configuration for convenience
func GetKMSConfig() *struct {
	Mode          string `yaml:"mode,default=embedded"`
	Endpoint      string `yaml:"endpoint,default=127.0.0.1:6820"`
	DataDir       string `yaml:"data_dir,default=/kms"`
	Binary        string `yaml:"binary,default=/usr/local/bin/progressdb-kms"`
	MasterKeyFile string `yaml:"master_key_file"`
	MasterKeyHex  string `yaml:"master_key_hex"`
} {
	configMu.RLock()
	defer configMu.RUnlock()
	if globalCfg == nil {
		return nil
	}
	// Convert to the expected return type
	kmsConfig := &struct {
		Mode          string `yaml:"mode,default=embedded"`
		Endpoint      string `yaml:"endpoint,default=127.0.0.1:6820"`
		DataDir       string `yaml:"data_dir,default=/kms"`
		Binary        string `yaml:"binary,default=/usr/local/bin/progressdb-kms"`
		MasterKeyFile string `yaml:"master_key_file"`
		MasterKeyHex  string `yaml:"master_key_hex"`
	}{}
	kmsConfig.Mode = globalCfg.Encryption.KMS.Mode
	kmsConfig.Endpoint = globalCfg.Encryption.KMS.Endpoint
	kmsConfig.DataDir = globalCfg.Encryption.KMS.DataDir
	kmsConfig.Binary = globalCfg.Encryption.KMS.Binary
	kmsConfig.MasterKeyFile = globalCfg.Encryption.KMS.MasterKeyFile
	kmsConfig.MasterKeyHex = globalCfg.Encryption.KMS.MasterKeyHex
	return kmsConfig
}

// LoadMasterKey loads the master key from configuration
func LoadMasterKey(cfg *Config) ([]byte, error) {
	kmsConfig := &cfg.Encryption.KMS

	// Try master key file first
	if kmsConfig.MasterKeyFile != "" {
		data, err := os.ReadFile(kmsConfig.MasterKeyFile)
		if err != nil {
			return nil, fmt.Errorf("reading master key file: %w", err)
		}
		// Remove any whitespace/newlines
		keyStr := strings.TrimSpace(string(data))
		return hex.DecodeString(keyStr)
	}

	// Try master key hex
	if kmsConfig.MasterKeyHex != "" {
		return hex.DecodeString(kmsConfig.MasterKeyHex)
	}

	return nil, fmt.Errorf("no master key configured: set either master_key_file or master_key_hex")
}
