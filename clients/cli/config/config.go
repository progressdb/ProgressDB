package config

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

// Config represents the migration configuration
type Config struct {
	OldEncryptionKey string `yaml:"old_encryption_key" json:"old_encryption_key"`
	FromDatabase     string `yaml:"from_database" json:"from_database"`
	ToDatabase       string `yaml:"to_database" json:"to_database"`
	OutputFormat     string `yaml:"output_format" json:"output_format"`
}

// LoadFromFile loads configuration from a YAML file
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

// SaveToFile saves configuration to a YAML file
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

// ValidateEncryptionKey validates the format of the encryption key
func ValidateEncryptionKey(key string) error {
	if key == "" {
		return fmt.Errorf("encryption key cannot be empty")
	}

	// Try to decode as hex
	decoded, err := hex.DecodeString(key)
	if err != nil {
		return fmt.Errorf("encryption key must be a valid hex string: %w", err)
	}

	if len(decoded) != 32 {
		return fmt.Errorf("encryption key must be exactly 32 bytes (64 hex characters), got %d bytes", len(decoded))
	}

	return nil
}

// IsComplete checks if all required configuration fields are present
func (c *Config) IsComplete() bool {
	return c.OldEncryptionKey != "" && c.FromDatabase != "" && c.ToDatabase != ""
}

// MissingFields returns a list of missing required fields
func (c *Config) MissingFields() []string {
	var missing []string
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
