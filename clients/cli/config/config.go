package config

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

type Config struct {
	OldEncryptionKey string            `yaml:"old_encryption_key" json:"old_encryption_key"`
	FromDatabase     string            `yaml:"from_database" json:"from_database"`
	ToDatabase       string            `yaml:"to_database" json:"to_database"`
	OutputFormat     string            `yaml:"output_format" json:"output_format"`
	OldConfigPath    string            `yaml:"old_config_path" json:"old_config_path"`
	OldDBPath        string            `yaml:"old_db_path" json:"old_db_path"`
	OldEncryptFields []OldEncryptField `yaml:"old_encrypt_fields" json:"old_encrypt_fields"`
}

type OldEncryptField struct {
	Path      string `yaml:"path" json:"path"`
	Algorithm string `yaml:"algorithm" json:"algorithm"`
}

func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

func SaveToFile(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func ValidateEncryptionKey(key string) error {
	if key == "" {
		return fmt.Errorf("encryption key cannot be empty")
	}

	decoded, err := hex.DecodeString(key)
	if err != nil {
		return fmt.Errorf("encryption key must be a valid hex string: %w", err)
	}

	if len(decoded) != 32 {
		return fmt.Errorf("encryption key must be exactly 32 bytes (64 hex characters), got %d bytes", len(decoded))
	}

	return nil
}

func (c *Config) IsComplete() bool {
	if c.OldConfigPath != "" {
		return true
	}
	return c.OldEncryptionKey != "" && c.FromDatabase != "" && c.ToDatabase != ""
}

func (c *Config) MissingFields() []string {
	var missing []string
	if c.OldConfigPath != "" {
		return missing
	}
	if c.OldEncryptionKey == "" {
		missing = append(missing, "old_encryption_key")
	}
	if c.FromDatabase == "" {
		missing = append(missing, "from_database")
	}
	if c.ToDatabase == "" {
		missing = append(missing, "to_database")
	}
	return missing
}

func (c *Config) LoadOldConfig(oldConfigPath string) error {
	data, err := os.ReadFile(oldConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read old config file: %w", err)
	}
	var oldCfg struct {
		Storage struct {
			DBPath string `yaml:"db_path"`
		} `yaml:"storage"`
		Security struct {
			EncryptionKey string            `yaml:"encryption_key"`
			Fields        []OldEncryptField `yaml:"fields"`
		} `yaml:"security"`
	}
	if err := yaml.Unmarshal(data, &oldCfg); err != nil {
		return fmt.Errorf("failed to parse old config file: %w", err)
	}
	if oldCfg.Storage.DBPath != "" {
		c.FromDatabase = oldCfg.Storage.DBPath
	}
	if oldCfg.Security.EncryptionKey != "" {
		c.OldEncryptionKey = oldCfg.Security.EncryptionKey
	}
	if len(oldCfg.Security.Fields) > 0 {
		c.OldEncryptFields = oldCfg.Security.Fields
	}
	c.OldConfigPath = oldConfigPath
	return nil
}
