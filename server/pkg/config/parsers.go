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
	Addr     string
	DB       string
	Config   string
	Set      map[string]bool
	Validate bool
}

// EnvResult holds the results of applying environment overrides.
type EnvResult struct {
	BackendKeys map[string]struct{}
	SigningKeys map[string]struct{}
	EnvUsed     bool
}

// EffectiveConfigResult holds the result of LoadEffectiveConfig.
type EffectiveConfigResult struct {
	Config *Config
	Addr   string
	DBPath string
	Source string // "flags", "config", or "env"
}

// ParseConfigFlags parses command-line flags and returns them as a Flags struct.
// you can only pass 3 config values
func ParseConfigFlags() Flags {
	addrPtr := flag.String("addr", ":8080", "HTTP listen address")
	dbPtr := flag.String("db", "./.database", "Pebble DB path")
	cfgPtr := flag.String("config", "./config.yaml", "Path to config file")
	// no validate flag; startup will always ensure state dirs
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
	// gather all relevant env variables up front
	envs := map[string]string{
		"SERVER_ADDR":             os.Getenv("PROGRESSDB_SERVER_ADDR"),
		"ADDR":                    os.Getenv("PROGRESSDB_ADDR"),
		"SERVER_ADDRESS":          os.Getenv("PROGRESSDB_SERVER_ADDRESS"),
		"SERVER_PORT":             os.Getenv("PROGRESSDB_SERVER_PORT"),
		"SERVER_DB_PATH":          os.Getenv("PROGRESSDB_SERVER_DB_PATH"),
		"DB_PATH":                 os.Getenv("PROGRESSDB_DB_PATH"),
		"ENCRYPTION_FIELDS":       os.Getenv("PROGRESSDB_ENCRYPTION_FIELDS"),
		"CORS_ORIGINS":            os.Getenv("PROGRESSDB_CORS_ORIGINS"),
		"RATE_RPS":                os.Getenv("PROGRESSDB_RATE_RPS"),
		"RATE_BURST":              os.Getenv("PROGRESSDB_RATE_BURST"),
		"IP_WHITELIST":            os.Getenv("PROGRESSDB_IP_WHITELIST"),
		"API_BACKEND_KEYS":        os.Getenv("PROGRESSDB_API_BACKEND_KEYS"),
		"API_FRONTEND_KEYS":       os.Getenv("PROGRESSDB_API_FRONTEND_KEYS"),
		"API_ADMIN_KEYS":          os.Getenv("PROGRESSDB_API_ADMIN_KEYS"),
		"KMS_ENDPOINT":            os.Getenv("PROGRESSDB_KMS_ENDPOINT"),
		"KMS_DATA_DIR":            os.Getenv("PROGRESSDB_KMS_DATA_DIR"),
		"KMS_MASTER_KEY_FILE":     os.Getenv("PROGRESSDB_KMS_MASTER_KEY_FILE"),
		"KMS_MASTER_KEY_HEX":      os.Getenv("PROGRESSDB_KMS_MASTER_KEY_HEX"),
		"USE_ENCRYPTION":          os.Getenv("PROGRESSDB_USE_ENCRYPTION"),
		"TLS_CERT":                os.Getenv("PROGRESSDB_TLS_CERT"),
		"TLS_KEY":                 os.Getenv("PROGRESSDB_TLS_KEY"),
		"RETENTION_ENABLED":       os.Getenv("PROGRESSDB_RETENTION_ENABLED"),
		"RETENTION_CRON":          os.Getenv("PROGRESSDB_RETENTION_CRON"),
		"RETENTION_PERIOD":        os.Getenv("PROGRESSDB_RETENTION_PERIOD"),
		"RETENTION_BATCH_SIZE":    os.Getenv("PROGRESSDB_RETENTION_BATCH_SIZE"),
		"RETENTION_BATCH_SLEEP_MS":os.Getenv("PROGRESSDB_RETENTION_BATCH_SLEEP_MS"),
		"RETENTION_DRY_RUN":       os.Getenv("PROGRESSDB_RETENTION_DRY_RUN"),
		"RETENTION_MIN_PERIOD":    os.Getenv("PROGRESSDB_RETENTION_MIN_PERIOD"),
	}

	// check if any env was set
	envUsed := false
	for _, v := range envs {
		if v != "" {
			envUsed = true
			break
		}
	}
	envCfg := &Config{}

	// parse helpers
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

	// apply env vars, giving precedence for address variables as per the original logic
	if v := envs["SERVER_ADDR"]; v != "" {
		if h, p, err := net.SplitHostPort(v); err == nil {
			envCfg.Server.Address = h
			if pi, err := strconv.Atoi(p); err == nil {
				envCfg.Server.Port = pi
			}
		} else {
			envCfg.Server.Address = v
		}
	} else if v := envs["ADDR"]; v != "" {
		if h, p, err := net.SplitHostPort(v); err == nil {
			envCfg.Server.Address = h
			if pi, err := strconv.Atoi(p); err == nil {
				envCfg.Server.Port = pi
			}
		} else {
			envCfg.Server.Address = v
		}
	} else {
		if host := envs["SERVER_ADDRESS"]; host != "" {
			envCfg.Server.Address = host
		}
		if port := envs["SERVER_PORT"]; port != "" {
			if pi, err := strconv.Atoi(port); err == nil {
				envCfg.Server.Port = pi
			}
		}
	}

	if v := envs["SERVER_DB_PATH"]; v != "" {
		envCfg.Server.DBPath = v
	} else if v := envs["DB_PATH"]; v != "" {
		envCfg.Server.DBPath = v
	}

	// encryption fields (now just []string, not []FieldEntry)
	if v := envs["ENCRYPTION_FIELDS"]; v != "" {
		envCfg.Security.Encryption.Fields = parseList(v)
	}

	if v := envs["CORS_ORIGINS"]; v != "" {
		envCfg.Security.CORS.AllowedOrigins = parseList(v)
	}
	if v := envs["RATE_RPS"]; v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			envCfg.Security.RateLimit.RPS = f
		}
	}
	if v := envs["RATE_BURST"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			envCfg.Security.RateLimit.Burst = n
		}
	}
	if v := envs["IP_WHITELIST"]; v != "" {
		envCfg.Security.IPWhitelist = parseList(v)
	}
	if v := envs["API_BACKEND_KEYS"]; v != "" {
		envCfg.Security.APIKeys.Backend = parseList(v)
	}
	if v := envs["API_FRONTEND_KEYS"]; v != "" {
		envCfg.Security.APIKeys.Frontend = parseList(v)
	}
	if v := envs["API_ADMIN_KEYS"]; v != "" {
		envCfg.Security.APIKeys.Admin = parseList(v)
	}

	// kms related env overrides
	if v := envs["KMS_ENDPOINT"]; v != "" {
		envCfg.Security.KMS.Endpoint = v
	}
	if v := envs["KMS_DATA_DIR"]; v != "" {
		envCfg.Security.KMS.DataDir = v
	}
	if v := envs["KMS_MASTER_KEY_FILE"]; v != "" {
		envCfg.Security.KMS.MasterKeyFile = v
	}
	if v := envs["KMS_MASTER_KEY_HEX"]; v != "" {
		envCfg.Security.KMS.MasterKeyHex = v
	}

	// encryption use flag
	if v := envs["USE_ENCRYPTION"]; v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes":
			envCfg.Security.Encryption.Use = true
		default:
			envCfg.Security.Encryption.Use = false
		}
	}

	// tls cert/key
	if c := envs["TLS_CERT"]; c != "" {
		envCfg.Server.TLS.CertFile = c
	}
	if k := envs["TLS_KEY"]; k != "" {
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

	// retention related env overrides
	if v := envs["RETENTION_ENABLED"]; v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes":
			envCfg.Retention.Enabled = true
		default:
			envCfg.Retention.Enabled = false
		}
	}
	if v := envs["RETENTION_CRON"]; v != "" {
		envCfg.Retention.Cron = v
	}
	if v := envs["RETENTION_PERIOD"]; v != "" {
		envCfg.Retention.Period = v
	}
	if v := envs["RETENTION_BATCH_SIZE"]; v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			envCfg.Retention.BatchSize = i
		}
	}
	if v := envs["RETENTION_BATCH_SLEEP_MS"]; v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			envCfg.Retention.BatchSleepMs = i
		}
	}
	if v := envs["RETENTION_DRY_RUN"]; v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes":
			envCfg.Retention.DryRun = true
		default:
			envCfg.Retention.DryRun = false
		}
	}
	if v := envs["RETENTION_MIN_PERIOD"]; v != "" {
		envCfg.Retention.MinPeriod = v
	}
	return envCfg, EnvResult{BackendKeys: backendKeys, SigningKeys: signingKeys, EnvUsed: envUsed}
}

// LoadEffectiveConfig decides which single source to use (flags, config
// file, or env) and returns the effective config plus resolved addr and
// dbPath. It honors an explicit flags.Config (user provided --config)
// by using the config file only; otherwise it uses flags if any flags
// are set; else if a config file exists it uses that; otherwise env.
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
