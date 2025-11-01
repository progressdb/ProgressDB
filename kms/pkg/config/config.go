package config

import (
	"fmt"
	"os"
	"strings"
)

var (
	// Global configuration loaded at init
	globalConfig *Config
	masterKey    string
)

// Config holds KMS configuration
type Config struct {
	DataDir string `yaml:"data_dir"`
}

// LoadConfig loads configuration from file or environment
func LoadConfig(configPath string) error {
	cfg := &Config{}

	// Try config file first
	if configPath != "" {
		if err := loadFromFile(configPath, cfg); err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}
	} else {
		// Try environment variables
		loadFromEnv(cfg)
	}

	// Set defaults
	if cfg.DataDir == "" {
		cfg.DataDir = "./kms"
	}

	globalConfig = cfg
	return nil
}

// GetConfig returns the loaded configuration
func GetConfig() *Config {
	return globalConfig
}

// GetMasterKey returns the loaded master key
func GetMasterKey() string {
	return masterKey
}

// LoadMasterKey loads master key from various sources
func LoadMasterKey(configPath string) error {
	// Try config file first
	if configPath != "" {
		if key, err := loadMasterKeyFromFile(configPath); err == nil && key != "" {
			masterKey = key
			return nil
		}
	}

	// Try environment variables
	if key := os.Getenv("KMS_MASTER_KEY"); key != "" {
		if err := ValidateMasterKey(key); err != nil {
			return fmt.Errorf("invalid KMS_MASTER_KEY: %w", err)
		}
		masterKey = key
		return nil
	}

	if key := os.Getenv("KMS_MASTER_KEY_HEX"); key != "" {
		if err := ValidateMasterKey(key); err != nil {
			return fmt.Errorf("invalid KMS_MASTER_KEY_HEX: %w", err)
		}
		masterKey = key
		return nil
	}

	if keyFile := os.Getenv("KMS_MASTER_KEY_FILE"); keyFile != "" {
		key, err := loadKeyFromFile(keyFile)
		if err != nil {
			return fmt.Errorf("failed to load master key from file %s: %w", keyFile, err)
		}
		masterKey = key
		return nil
	}

	return fmt.Errorf("no master key found: set KMS_MASTER_KEY, KMS_MASTER_KEY_HEX, or KMS_MASTER_KEY_FILE")
}

// loadFromFile loads config from YAML file
func loadFromFile(path string, cfg *Config) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return parseYAML(b, cfg)
}

// loadFromEnv loads config from environment variables
func loadFromEnv(cfg *Config) {
	if dataDir := os.Getenv("KMS_DATA_DIR"); dataDir != "" {
		cfg.DataDir = dataDir
	}
}

// loadMasterKeyFromFile loads master key from config file
func loadMasterKeyFromFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var config struct {
		MasterKey     string `yaml:"master_key"`
		MasterKeyHex  string `yaml:"master_key_hex"`
		MasterKeyFile string `yaml:"master_key_file"`
	}

	if err := parseYAML(b, &config); err != nil {
		return "", err
	}

	// Check direct hex key first
	if config.MasterKeyHex != "" {
		if err := ValidateMasterKey(config.MasterKeyHex); err != nil {
			return "", fmt.Errorf("invalid master_key_hex: %w", err)
		}
		return config.MasterKeyHex, nil
	}

	// Check fallback master key
	if config.MasterKey != "" {
		if err := ValidateMasterKey(config.MasterKey); err != nil {
			return "", fmt.Errorf("invalid master_key: %w", err)
		}
		return config.MasterKey, nil
	}

	// Check master key file
	if config.MasterKeyFile != "" {
		return loadKeyFromFile(config.MasterKeyFile)
	}

	return "", nil
}

// loadKeyFromFile loads key from a file
func loadKeyFromFile(path string) (string, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(string(keyBytes))
	if err := ValidateMasterKey(key); err != nil {
		return "", fmt.Errorf("invalid master key in file %s: %w", path, err)
	}
	return key, nil
}

// parseYAML parses simple YAML
func parseYAML(data []byte, v interface{}) error {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Use reflection or simple switch for known fields
		switch cfg := v.(type) {
		case *Config:
			if key == "data_dir" {
				cfg.DataDir = value
			}
		case *struct {
			MasterKey     string `yaml:"master_key"`
			MasterKeyHex  string `yaml:"master_key_hex"`
			MasterKeyFile string `yaml:"master_key_file"`
		}:
			if key == "master_key" {
				cfg.MasterKey = value
			} else if key == "master_key_hex" {
				cfg.MasterKeyHex = value
			} else if key == "master_key_file" {
				cfg.MasterKeyFile = value
			}
		}
	}
	return nil
}
