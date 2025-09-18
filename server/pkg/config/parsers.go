package config

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// Flags holds parsed command-line flag values and which were set.
type Flags struct {
	Addr   string
	DB     string
	Config string
	Set    map[string]bool
}

// EnvResult holds the results of applying environment overrides.
type EnvResult struct {
	BackendKeys map[string]struct{}
	SigningKeys map[string]struct{}
	EnvUsed     bool
}

// LoadEffectiveConfig decides which single source to use (flags, config
// file, or env) and returns an EffectiveConfigResult plus error.
type EffectiveConfigResult struct {
	Config *Config
	Addr   string
	DBPath string
	Source string // "flags", "config", or "env"
}

// ParseConfigFlags parses command-line flags and returns them as a Flags struct.
func ParseConfigFlags() Flags {
	addrPtr := flag.String("addr", ":8080", "HTTP listen address")
	dbPtr := flag.String("db", "./.database", "Pebble DB path")
	cfgPtr := flag.String("config", "./config.yaml", "Path to config file")
	flag.Parse()
	setFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })
	return Flags{Addr: *addrPtr, DB: *dbPtr, Config: *cfgPtr, Set: setFlags}
}

// ParseConfigFile resolves the config path and loads the YAML file. It
// returns the parsed config, a boolean indicating whether the file was
// present, and an error for fatal parsing problems.
func ParseConfigFile(flags Flags) (*Config, bool, error) {
	cfgPath := ResolveConfigPath(flags.Config, flags.Set["config"])
	cfg, err := Load(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, false, nil
		}
		return nil, false, err
	}
	return cfg, true, nil
}

// ParseConfigEnvs reads environment variables into a fresh Config and
// returns that env-only config plus an EnvResult describing keys present
// and whether envs were used. This function does not mutate any caller
// provided config.
func ParseConfigEnvs() (*Config, EnvResult) {
	envCfg := &Config{}
	envUsed := false
	parseList := func(v string) []string {
		if v == "" {
			return nil
		}
		parts := []string{}
		for _, p := range strings.Split(v, ",") {
			if s := strings.TrimSpace(p); s != "" {
				parts = append(parts, s)
			}
		}
		return parts
	}

	// Server address/port
	if v := os.Getenv("PROGRESSDB_SERVER_ADDR"); v != "" {
		envUsed = true
		if h, p, err := net.SplitHostPort(v); err == nil {
			envCfg.Server.Address = h
			if pi, err := strconv.Atoi(p); err == nil {
				envCfg.Server.Port = pi
			}
		} else {
			envCfg.Server.Address = v
		}
	} else if v := os.Getenv("PROGRESSDB_ADDR"); v != "" {
		envUsed = true
		if h, p, err := net.SplitHostPort(v); err == nil {
			envCfg.Server.Address = h
			if pi, err := strconv.Atoi(p); err == nil {
				envCfg.Server.Port = pi
			}
		} else {
			envCfg.Server.Address = v
		}
	} else {
		if host := os.Getenv("PROGRESSDB_SERVER_ADDRESS"); host != "" {
			envUsed = true
			envCfg.Server.Address = host
		}
		if port := os.Getenv("PROGRESSDB_SERVER_PORT"); port != "" {
			envUsed = true
			if pi, err := strconv.Atoi(port); err == nil {
				envCfg.Server.Port = pi
			}
		}
	}

	if v := os.Getenv("PROGRESSDB_SERVER_DB_PATH"); v != "" {
		envUsed = true
		envCfg.Server.DBPath = v
	} else if v := os.Getenv("PROGRESSDB_DB_PATH"); v != "" {
		envUsed = true
		envCfg.Server.DBPath = v
	}

	// Encryption fields
	if v := os.Getenv("PROGRESSDB_ENCRYPTION_FIELDS"); v != "" {
		envUsed = true
		parts := parseList(v)
		envCfg.Security.Fields = nil
		envCfg.Security.Encryption.Fields = nil
		for _, p := range parts {
			entry := FieldEntry{Path: p, Algorithm: "aes-gcm"}
			envCfg.Security.Fields = append(envCfg.Security.Fields, entry)
			envCfg.Security.Encryption.Fields = append(envCfg.Security.Encryption.Fields, entry)
		}
	}

	if v := os.Getenv("PROGRESSDB_CORS_ORIGINS"); v != "" {
		envUsed = true
		envCfg.Security.CORS.AllowedOrigins = parseList(v)
	}
	if v := os.Getenv("PROGRESSDB_RATE_RPS"); v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			envUsed = true
			envCfg.Security.RateLimit.RPS = f
		}
	}
	if v := os.Getenv("PROGRESSDB_RATE_BURST"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			envUsed = true
			envCfg.Security.RateLimit.Burst = n
		}
	}
	if v := os.Getenv("PROGRESSDB_IP_WHITELIST"); v != "" {
		envUsed = true
		envCfg.Security.IPWhitelist = parseList(v)
	}
	if v := os.Getenv("PROGRESSDB_API_BACKEND_KEYS"); v != "" {
		envUsed = true
		envCfg.Security.APIKeys.Backend = parseList(v)
	}
	if v := os.Getenv("PROGRESSDB_API_FRONTEND_KEYS"); v != "" {
		envUsed = true
		envCfg.Security.APIKeys.Frontend = parseList(v)
	}
	if v := os.Getenv("PROGRESSDB_API_ADMIN_KEYS"); v != "" {
		envUsed = true
		envCfg.Security.APIKeys.Admin = parseList(v)
	}

	// KMS related env overrides
	if v := os.Getenv("PROGRESSDB_KMS_ENDPOINT"); v != "" {
		envUsed = true
		envCfg.Security.KMS.Endpoint = v
	}
	if v := os.Getenv("PROGRESSDB_KMS_DATA_DIR"); v != "" {
		envUsed = true
		envCfg.Security.KMS.DataDir = v
	}
	// PROGRESSDB_KMS_BINARY is intentionally not supported; the server
	// discovers and spawns the KMS binary from PATH or alongside the
	// server executable. Operators who wish to run a custom binary should
	// run it themselves or modify the server launcher.
	if v := os.Getenv("PROGRESSDB_KMS_MASTER_KEY_FILE"); v != "" {
		envUsed = true
		envCfg.Security.KMS.MasterKeyFile = v
	}
	if v := os.Getenv("PROGRESSDB_KMS_MASTER_KEY_HEX"); v != "" {
		envUsed = true
		envCfg.Security.KMS.MasterKeyHex = v
	}

	// Encryption use flag
	if v := os.Getenv("PROGRESSDB_USE_ENCRYPTION"); v != "" {
		envUsed = true
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes":
			envCfg.Security.Encryption.Use = true
		default:
			envCfg.Security.Encryption.Use = false
		}
	}

	// TLS cert/key
	if c := os.Getenv("PROGRESSDB_TLS_CERT"); c != "" {
		envUsed = true
		envCfg.Server.TLS.CertFile = c
	}
	if k := os.Getenv("PROGRESSDB_TLS_KEY"); k != "" {
		envUsed = true
		envCfg.Server.TLS.KeyFile = k
	}

	backendKeys := make(map[string]struct{})
	for _, k := range envCfg.Security.APIKeys.Backend {
		backendKeys[k] = struct{}{}
	}
	signingKeys := make(map[string]struct{})
	for k := range backendKeys {
		signingKeys[k] = struct{}{}
	}
	return envCfg, EnvResult{BackendKeys: backendKeys, SigningKeys: signingKeys, EnvUsed: envUsed}
}

// LoadEffectiveConfig decides which single source to use (flags, config
// file, or env) and returns the effective config plus resolved addr and
// dbPath. It honors an explicit flags.Config (user provided --config)
// by using the config file only; otherwise it uses flags if any flags
// are set; else if a config file exists it uses that; otherwise env.
// EffectiveConfigResult holds the result of LoadEffectiveConfig.
func LoadEffectiveConfig(flags Flags, fileCfg *Config, fileExists bool, envCfg *Config, envRes EnvResult) (EffectiveConfigResult, error) {
	var res EffectiveConfigResult

	// If user explicitly passed --config, require the file to exist and use it.
	if flags.Set["config"] {
		if !fileExists {
			return res, fmt.Errorf("config file %s not found", flags.Config)
		}
		res.Config = fileCfg
		res.Addr = fileCfg.Addr()
		res.DBPath = fileCfg.Server.DBPath
		res.Source = "config"
		return res, nil
	}

	// If user passed any non-config flags (addr/db), use flags exclusively.
	if flags.Set["addr"] || flags.Set["db"] {
		addr := flags.Addr
		if !flags.Set["addr"] {
			addr = envCfg.Addr()
			if addr == "" {
				addr = fileCfg.Addr()
			}
		}
		dbPath := flags.DB
		if !flags.Set["db"] {
			if p := strings.TrimSpace(envCfg.Server.DBPath); p != "" {
				dbPath = p
			} else if p := strings.TrimSpace(fileCfg.Server.DBPath); p != "" {
				dbPath = p
			}
		}
		out := &Config{}
		out.Server.Address = addr
		out.Server.Port = parsePortFromAddr(addr)
		out.Server.DBPath = dbPath
		res.Config = out
		res.Addr = addr
		res.DBPath = dbPath
		res.Source = "flags"
		return res, nil
	}

	// No explicit flags: prefer file config if present, otherwise env.
	if fileExists {
		res.Config = fileCfg
		res.Addr = fileCfg.Addr()
		res.DBPath = fileCfg.Server.DBPath
		res.Source = "config"
		return res, nil
	}
	// fallback to env
	res.Config = envCfg
	res.Addr = envCfg.Addr()
	res.DBPath = envCfg.Server.DBPath
	res.Source = "env"
	return res, nil
}

// parsePortFromAddr extracts port integer from host:port string.
func parsePortFromAddr(a string) int {
	if a == "" {
		return 0
	}
	if _, p, err := net.SplitHostPort(a); err == nil {
		if pi, err := strconv.Atoi(p); err == nil {
			return pi
		}
	}
	return 0
}
