package config

import (
	"os"
)

type Config struct {
	Endpoint     string `yaml:"endpoint"`
	DataDir      string `yaml:"data_dir"`
	MasterKey    string `yaml:"master_key"`
	MasterKeyHex string `yaml:"master_key_hex"`
}

func DefaultConfig() *Config {
	return &Config{
		Endpoint: "127.0.0.1:6820",
		DataDir:  "./kms-data",
	}
}

func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := parseYAML(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.DataDir == "" {
		c.DataDir = "./kms-data"
	}

	if c.Endpoint == "" {
		c.Endpoint = "127.0.0.1:6820"
	}

	return nil
}

func (c *Config) GetMasterKey() string {
	if c.MasterKeyHex != "" {
		return c.MasterKeyHex
	}
	return c.MasterKey
}

func (c *Config) HasMasterKey() bool {
	return c.MasterKey != "" || c.MasterKeyHex != ""
}
