package config

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

func parseYAML(data []byte, cfg *Config) error {
	return yaml.Unmarshal(data, cfg)
}

func ValidateConfigPath(cfgPath string) error {
	if cfgPath == "" {
		return nil
	}

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return err
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		return err
	}

	if info.Mode().Perm()&0004 != 0 {
		log.Printf("WARNING: Config file %s is world-readable. Consider restricting permissions.", cfgPath)
	}

	return nil
}

// LoadMasterKeyFromConfig loads and validates master key from YAML config
func LoadMasterKeyFromConfig(cfgPath string) (string, error) {
	// Validate config file path and permissions
	if err := ValidateConfigPath(cfgPath); err != nil {
		return "", fmt.Errorf("config file does not exist: %s", cfgPath)
	}

	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	var config struct {
		MasterKeyHex string `yaml:"master_key_hex"`
		MasterKey    string `yaml:"master_key"`
	}

	if err := yaml.Unmarshal(b, &config); err != nil {
		return "", fmt.Errorf("failed to parse config YAML: %w", err)
	}

	masterKey := config.MasterKeyHex
	if masterKey == "" {
		masterKey = config.MasterKey
	}

	if masterKey == "" {
		return "", fmt.Errorf("no master_key or master_key_hex found in config")
	}

	// Validate hex format and key strength
	if err := ValidateMasterKey(masterKey); err != nil {
		return "", fmt.Errorf("master key failed validation: %w", err)
	}

	return masterKey, nil
}
