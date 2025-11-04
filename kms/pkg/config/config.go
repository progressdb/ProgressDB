package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds the essential KMS configuration
type Config struct {
	KMS KMSConfig `yaml:"kms"`
}

// KMSConfig holds KMS specific settings
type KMSConfig struct {
	DBPath        string `yaml:"db_path,default=/kms"`
	MasterKeyFile string `yaml:"master_key_file"`
	MasterKeyHex  string `yaml:"master_key_hex"`
}

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

	// Set defaults
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
	if DBPath := os.Getenv("KMS_DATA_DIR"); DBPath != "" {
		cfg.KMS.DBPath = DBPath
	}
	if masterKeyFile := os.Getenv("KMS_MASTER_KEY_FILE"); masterKeyFile != "" {
		cfg.KMS.MasterKeyFile = masterKeyFile
	}
	if masterKeyHex := os.Getenv("KMS_MASTER_KEY_HEX"); masterKeyHex != "" {
		cfg.KMS.MasterKeyHex = masterKeyHex
	}
}

// setDefaults sets default values for empty configuration fields
func setDefaults(cfg *Config) {
	if cfg.KMS.DBPath == "" {
		cfg.KMS.DBPath = "/kms"
	}
}

// LoadMasterKey loads the master key from configuration
func LoadMasterKey(cfg *Config) ([]byte, error) {
	// Try master key file first
	if cfg.KMS.MasterKeyFile != "" {
		data, err := os.ReadFile(cfg.KMS.MasterKeyFile)
		if err != nil {
			return nil, fmt.Errorf("reading master key file: %w", err)
		}
		// Remove any whitespace/newlines
		keyStr := strings.TrimSpace(string(data))
		return hex.DecodeString(keyStr)
	}

	// Try master key hex
	if cfg.KMS.MasterKeyHex != "" {
		return hex.DecodeString(cfg.KMS.MasterKeyHex)
	}

	return nil, fmt.Errorf("no master key configured: set either master_key_file or master_key_hex")
}
