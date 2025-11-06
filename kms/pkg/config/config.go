package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	KMS KMSConfig `yaml:"kms"`
}

type KMSConfig struct {
	DBPath        string `yaml:"db_path"`
	MasterKeyFile string `yaml:"master_key_file"`
	MasterKeyHex  string `yaml:"master_key_hex"`
}

func LoadConfig(configPath string) (*Config, error) {
	cfg := &Config{}

	if configPath != "" {
		if err := loadFromFile(configPath, cfg); err != nil {
			return nil, fmt.Errorf("failed to load config from file: %w", err)
		}
	}

	overrideWithEnv(cfg)
	setDefaults(cfg)
	return cfg, nil
}

func loadFromFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}

	return nil
}

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

func setDefaults(cfg *Config) {
	if cfg.KMS.DBPath == "" {
		cfg.KMS.DBPath = "/kms"
	}
}

func LoadMasterKey(cfg *Config) ([]byte, error) {
	if cfg.KMS.MasterKeyFile != "" {
		data, err := os.ReadFile(cfg.KMS.MasterKeyFile)
		if err != nil {
			return nil, fmt.Errorf("reading master key file: %w", err)
		}
		keyStr := strings.TrimSpace(string(data))
		return hex.DecodeString(keyStr)
	}

	if cfg.KMS.MasterKeyHex != "" {
		return hex.DecodeString(cfg.KMS.MasterKeyHex)
	}

	return nil, fmt.Errorf("no master key configured: set either master_key_file or master_key_hex")
}
