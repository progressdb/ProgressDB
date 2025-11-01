package config

import (
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
	}

	return nil
}
