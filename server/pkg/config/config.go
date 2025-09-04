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
        TLS struct {
            CertFile string `yaml:"cert_file"`
            KeyFile  string `yaml:"key_file"`
        } `yaml:"tls"`
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
        CORS struct {
            AllowedOrigins []string `yaml:"allowed_origins"`
        } `yaml:"cors"`
        RateLimit struct {
            RPS   float64 `yaml:"rps"`
            Burst int     `yaml:"burst"`
        } `yaml:"rate_limit"`
        IPWhitelist []string `yaml:"ip_whitelist"`
        APIKeys struct {
            Backend  []string `yaml:"backend"`
            Frontend []string `yaml:"frontend"`
            AllowUnauth bool  `yaml:"allow_unauth"`
        } `yaml:"api_keys"`
    } `yaml:"security"`
    Logging struct {
        Level  string `yaml:"level"`
        Format string `yaml:"format"` // text|json
        HTTP   struct {
            Enabled bool   `yaml:"enabled"`
            URL     string `yaml:"url"`
            Bearer  string `yaml:"bearer"`
        } `yaml:"http"`
    } `yaml:"logging"`
    Validation struct {
        Required []string `yaml:"required"`
        Types    []struct {
            Path string `yaml:"path"`
            Type string `yaml:"type"` // string|number|boolean|object|array
        } `yaml:"types"`
        MaxLen []struct {
            Path string `yaml:"path"`
            Max  int    `yaml:"max"`
        } `yaml:"max_len"`
        Enums []struct {
            Path   string   `yaml:"path"`
            Values []string `yaml:"values"`
        } `yaml:"enums"`
        WhenThen []struct {
            When struct {
                Path   string      `yaml:"path"`
                Equals interface{} `yaml:"equals"`
            } `yaml:"when"`
            Then struct {
                Required []string `yaml:"required"`
            } `yaml:"then"`
        } `yaml:"when_then"`
    } `yaml:"validation"`
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
