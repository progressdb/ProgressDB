package config

import (
    "fmt"
    "io/ioutil"
    "os"

    "gopkg.in/yaml.v3"
)

type Config struct {
    Server struct {
        Address string `yaml:"address"`
        Port    int    `yaml:"port"`
    } `yaml:"server"`
    Storage struct {
        DBPath string `yaml:"db_path"`
    } `yaml:"storage"`
    Security struct {
        EncryptionKey string `yaml:"encryption_key"`
        Fields        []struct {
            Path      string `yaml:"path"`
            Algorithm string `yaml:"algorithm"`
        } `yaml:"fields"`
    } `yaml:"security"`
    Logging struct {
        Level string `yaml:"level"`
    } `yaml:"logging"`
}

// Addr returns host:port for HTTP server.
func (c *Config) Addr() string {
    addr := c.Server.Address
    if addr == "" {
        addr = "0.0.0.0"
    }
    p := c.Server.Port
    if p == 0 {
        p = 8080
    }
    return fmt.Sprintf("%s:%d", addr, p)
}

func Load(path string) (*Config, error) {
    b, err := ioutil.ReadFile(path)
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
