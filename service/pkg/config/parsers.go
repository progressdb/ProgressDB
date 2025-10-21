package config

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// holds parsed command-line flag values and which were set
type Flags struct {
	Addr     string
	DB       string
	Config   string
	Set      map[string]bool
	Validate bool
}

// holds the results of applying environment overrides
type EnvResult struct {
	BackendKeys map[string]struct{}
	SigningKeys map[string]struct{}
	EnvUsed     bool
}

// holds the result of loadEffectiveConfig
type EffectiveConfigResult struct {
	Config *Config
	Addr   string
	DBPath string
	Source string // "flags", "config", or "env"
}

// parses command-line flags and returns them as a Flags struct
// you can only pass 3 config values
func ParseConfigFlags() Flags {
	// parse any flags with defaults
	addrPtr := flag.String("addr", ":8080", "HTTP listen address")
	dbPtr := flag.String("db", "./.database", "Pebble DB path")
	cfgPtr := flag.String("config", "./config.yaml", "Path to config file")
	flag.Parse()

	// record which flags were set explicitly
	setFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })

	// return with defaults
	return Flags{Addr: *addrPtr, DB: *dbPtr, Config: *cfgPtr, Set: setFlags}
}

// loads config from file, returns config, found bool, and error
func ParseConfigFile(flags Flags) (*Config, bool, error) {
	cfgPath := ResolveConfigPath(flags.Config, flags.Set["config"])
	cfg, err := LoadConfigFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, false, nil
		}
		return nil, false, err
	}
	return cfg, true, nil
}

// loads environment variables into a new Config and returns it with EnvResult; caller config is unchanged
func ParseConfigEnvs() (*Config, EnvResult) {
	// gather all relevant env variables
	envs := map[string]string{
		"SERVER_ADDR":         os.Getenv("PROGRESSDB_SERVER_ADDR"),
		"ADDR":                os.Getenv("PROGRESSDB_ADDR"),
		"SERVER_ADDRESS":      os.Getenv("PROGRESSDB_SERVER_ADDRESS"),
		"SERVER_PORT":         os.Getenv("PROGRESSDB_SERVER_PORT"),
		"SERVER_DB_PATH":      os.Getenv("PROGRESSDB_SERVER_DB_PATH"),
		"DB_PATH":             os.Getenv("PROGRESSDB_DB_PATH"),
		"ENCRYPTION_FIELDS":   os.Getenv("PROGRESSDB_ENCRYPTION_FIELDS"),
		"CORS_ORIGINS":        os.Getenv("PROGRESSDB_CORS_ORIGINS"),
		"RATE_RPS":            os.Getenv("PROGRESSDB_RATE_RPS"),
		"RATE_BURST":          os.Getenv("PROGRESSDB_RATE_BURST"),
		"IP_WHITELIST":        os.Getenv("PROGRESSDB_IP_WHITELIST"),
		"API_BACKEND_KEYS":    os.Getenv("PROGRESSDB_API_BACKEND_KEYS"),
		"API_FRONTEND_KEYS":   os.Getenv("PROGRESSDB_API_FRONTEND_KEYS"),
		"API_ADMIN_KEYS":      os.Getenv("PROGRESSDB_API_ADMIN_KEYS"),
		"KMS_MODE":            os.Getenv("PROGRESSDB_KMS_MODE"),
		"KMS_ENDPOINT":        os.Getenv("PROGRESSDB_KMS_ENDPOINT"),
		"KMS_DATA_DIR":        os.Getenv("PROGRESSDB_KMS_DATA_DIR"),
		"KMS_MASTER_KEY_FILE": os.Getenv("PROGRESSDB_KMS_MASTER_KEY_FILE"),
		"KMS_MASTER_KEY_HEX":  os.Getenv("PROGRESSDB_KMS_MASTER_KEY_HEX"),
		"ENCRYPTION_ENABLED":  os.Getenv("PROGRESSDB_ENCRYPTION_ENABLED"),
		"TLS_CERT":            os.Getenv("PROGRESSDB_TLS_CERT"),
		"TLS_KEY":             os.Getenv("PROGRESSDB_TLS_KEY"),

		// data retentioon feature
		"RETENTION_ENABLED":        os.Getenv("PROGRESSDB_RETENTION_ENABLED"),
		"RETENTION_CRON":           os.Getenv("PROGRESSDB_RETENTION_CRON"),
		"RETENTION_PERIOD":         os.Getenv("PROGRESSDB_RETENTION_PERIOD"),
		"RETENTION_BATCH_SIZE":     os.Getenv("PROGRESSDB_RETENTION_BATCH_SIZE"),
		"RETENTION_BATCH_SLEEP_MS": os.Getenv("PROGRESSDB_RETENTION_BATCH_SLEEP_MS"),
		"RETENTION_DRY_RUN":        os.Getenv("PROGRESSDB_RETENTION_DRY_RUN"),
		"RETENTION_MIN_PERIOD":     os.Getenv("PROGRESSDB_RETENTION_MIN_PERIOD"),

		// retention lock TTL
		"RETENTION_LOCK_TTL": os.Getenv("PROGRESSDB_RETENTION_LOCK_TTL"),

		// telemetry
		"TELEMETRY_SAMPLE_RATE":    os.Getenv("PROGRESSDB_TELEMETRY_SAMPLE_RATE"),
		"TELEMETRY_SLOW_THRESHOLD": os.Getenv("PROGRESSDB_TELEMETRY_SLOW_THRESHOLD"),
		"TELEMETRY_BUFFER_SIZE":    os.Getenv("PROGRESSDB_TELEMETRY_BUFFER_SIZE"),
		"TELEMETRY_FILE_MAX_SIZE":  os.Getenv("PROGRESSDB_TELEMETRY_FILE_MAX_SIZE"),
		"TELEMETRY_FLUSH_INTERVAL": os.Getenv("PROGRESSDB_TELEMETRY_FLUSH_INTERVAL"),
		"TELEMETRY_QUEUE_CAPACITY": os.Getenv("PROGRESSDB_TELEMETRY_QUEUE_CAPACITY"),

		// sensor.monitor
		"SENSOR_MONITOR_POLL_INTERVAL":   os.Getenv("PROGRESSDB_SENSOR_MONITOR_POLL_INTERVAL"),
		"SENSOR_MONITOR_DISK_HIGH_PCT":   os.Getenv("PROGRESSDB_SENSOR_MONITOR_DISK_HIGH_PCT"),
		"SENSOR_MONITOR_DISK_LOW_PCT":    os.Getenv("PROGRESSDB_SENSOR_MONITOR_DISK_LOW_PCT"),
		"SENSOR_MONITOR_MEM_HIGH_PCT":    os.Getenv("PROGRESSDB_SENSOR_MONITOR_MEM_HIGH_PCT"),
		"SENSOR_MONITOR_CPU_HIGH_PCT":    os.Getenv("PROGRESSDB_SENSOR_MONITOR_CPU_HIGH_PCT"),
		"SENSOR_MONITOR_RECOVERY_WINDOW": os.Getenv("PROGRESSDB_SENSOR_MONITOR_RECOVERY_WINDOW"),

		// logging
		"LOG_LEVEL": os.Getenv("PROGRESSDB_LOG_LEVEL"),

		// intake
		"INTAKE_BUFFER_CAPACITY":        os.Getenv("PROGRESSDB_INTAKE_BUFFER_CAPACITY"),
		"INTAKE_SHUTDOWN_POLL_INTERVAL": os.Getenv("PROGRESSDB_INTAKE_SHUTDOWN_POLL_INTERVAL"),
		"INTAKE_WAL_ENABLED":            os.Getenv("PROGRESSDB_INTAKE_WAL_ENABLED"),
		"INTAKE_WAL_SEGMENT_SIZE":       os.Getenv("PROGRESSDB_INTAKE_WAL_SEGMENT_SIZE"),
		// compute
		"COMPUTE_WORKER_COUNT":    os.Getenv("PROGRESSDB_COMPUTE_WORKER_COUNT"),
		"COMPUTE_BUFFER_CAPACITY": os.Getenv("PROGRESSDB_COMPUTE_BUFFER_CAPACITY"),
		// apply
		"APPLY_BATCH_COUNT":   os.Getenv("PROGRESSDB_APPLY_BATCH_COUNT"),
		"APPLY_BATCH_TIMEOUT": os.Getenv("PROGRESSDB_APPLY_BATCH_TIMEOUT"),
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

	parseBool := func(v string, def bool) bool {
		if v == "" {
			return def
		}
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes":
			return true
		default:
			return false
		}
	}

	// parse size and duration helpers for env values
	parseSizeBytes := func(v string) SizeBytes {
		if strings.TrimSpace(v) == "" {
			return SizeBytes(0)
		}
		if u, err := humanize.ParseBytes(v); err == nil {
			return SizeBytes(u)
		}
		if i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return SizeBytes(i)
		}
		return SizeBytes(0)
	}

	parseDuration := func(v string) Duration {
		if strings.TrimSpace(v) == "" {
			return Duration(0)
		}
		if td, err := time.ParseDuration(v); err == nil {
			return Duration(td)
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return Duration(time.Duration(f * float64(time.Second)))
		}
		return Duration(0)
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

	// encryption fields ([]string)
	if v := envs["ENCRYPTION_FIELDS"]; v != "" {
		envCfg.Encryption.Fields = parseList(v)
	}

	if v := envs["CORS_ORIGINS"]; v != "" {
		envCfg.Server.CORS.AllowedOrigins = parseList(v)
	}
	if v := envs["RATE_RPS"]; v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			envCfg.Server.RateLimit.RPS = f
		}
	}
	if v := envs["RATE_BURST"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			envCfg.Server.RateLimit.Burst = n
		}
	}
	if v := envs["IP_WHITELIST"]; v != "" {
		envCfg.Server.IPWhitelist = parseList(v)
	}
	if v := envs["API_BACKEND_KEYS"]; v != "" {
		envCfg.Server.APIKeys.Backend = parseList(v)
	}
	if v := envs["API_FRONTEND_KEYS"]; v != "" {
		envCfg.Server.APIKeys.Frontend = parseList(v)
	}
	if v := envs["API_ADMIN_KEYS"]; v != "" {
		envCfg.Server.APIKeys.Admin = parseList(v)
	}

	// kms related env overrides
	if v := envs["KMS_MODE"]; v != "" {
		envCfg.Encryption.KMS.Mode = v
	}
	if v := envs["KMS_ENDPOINT"]; v != "" {
		envCfg.Encryption.KMS.Endpoint = v
	}
	if v := envs["KMS_DATA_DIR"]; v != "" {
		envCfg.Encryption.KMS.DataDir = v
	}
	if v := envs["KMS_MASTER_KEY_FILE"]; v != "" {
		envCfg.Encryption.KMS.MasterKeyFile = v
	}
	if v := envs["KMS_MASTER_KEY_HEX"]; v != "" {
		envCfg.Encryption.KMS.MasterKeyHex = v
	}

	// encryption enabled flag
	if v := envs["ENCRYPTION_ENABLED"]; v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes":
			envCfg.Encryption.Enabled = true
		default:
			envCfg.Encryption.Enabled = false
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
	for _, k := range envCfg.Server.APIKeys.Backend {
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

	// retention lock TTL
	if v := envs["RETENTION_LOCK_TTL"]; v != "" {
		envCfg.Retention.LockTTL = parseDuration(v)
	}

	// telemetry env overrides
	if v := envs["TELEMETRY_SAMPLE_RATE"]; v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			envCfg.Telemetry.SampleRate = f
		}
	}
	if v := envs["TELEMETRY_SLOW_THRESHOLD"]; v != "" {
		envCfg.Telemetry.SlowThreshold = parseDuration(v)
	}

	// sensor.monitor env overrides
	if v := envs["SENSOR_MONITOR_POLL_INTERVAL"]; v != "" {
		envCfg.Sensor.Monitor.PollInterval = parseDuration(v)
	}
	if v := envs["SENSOR_MONITOR_DISK_HIGH_PCT"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			envCfg.Sensor.Monitor.DiskHighPct = n
		}
	}
	if v := envs["SENSOR_MONITOR_DISK_LOW_PCT"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			envCfg.Sensor.Monitor.DiskLowPct = n
		}
	}
	if v := envs["SENSOR_MONITOR_MEM_HIGH_PCT"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			envCfg.Sensor.Monitor.MemHighPct = n
		}
	}
	if v := envs["SENSOR_MONITOR_RECOVERY_WINDOW"]; v != "" {
		envCfg.Sensor.Monitor.RecoveryWindow = parseDuration(v)
	}

	// logging env overrides
	if v := envs["LOG_LEVEL"]; v != "" {
		envCfg.Logging.Level = strings.TrimSpace(v)
	}

	if v := envs["INTAKE_QUEUE_CAPACITY"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			envCfg.Ingest.Intake.QueueCapacity = n
		}
	}
	if v := envs["INTAKE_SHUTDOWN_POLL_INTERVAL"]; v != "" {
		envCfg.Ingest.Intake.ShutdownPollInterval = parseDuration(v)
	}
	if v := envs["INTAKE_WAL_ENABLED"]; v != "" {
		envCfg.Ingest.Intake.WAL.Enabled = parseBool(v, true)
	}
	if v := envs["INTAKE_WAL_SEGMENT_SIZE"]; v != "" {
		envCfg.Ingest.Intake.WAL.SegmentSize = parseSizeBytes(v)
	}
	if v := envs["COMPUTE_WORKER_COUNT"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			envCfg.Ingest.Compute.WorkerCount = n
		}
	}

	if v := envs["COMPUTE_BUFFER_CAPACITY"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			envCfg.Ingest.Compute.BufferCapacity = n
		}
	}
	if v := envs["APPLY_BATCH_COUNT"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			envCfg.Ingest.Apply.BatchCount = n
		}
	}
	if v := envs["APPLY_BATCH_TIMEOUT"]; v != "" {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil {
			envCfg.Ingest.Apply.BatchTimeout = Duration(d)
		}
	}
	return envCfg, EnvResult{BackendKeys: backendKeys, SigningKeys: signingKeys, EnvUsed: envUsed}
}

// decides which single source to use (flags, config file, or env) and returns the effective config plus resolved addr and dbPath. if --config is set, only the config file is used; otherwise flags if set; else config file if present; else env
func LoadEffectiveConfig(flags Flags, fileCfg *Config, fileExists bool, envCfg *Config, envRes EnvResult) (EffectiveConfigResult, error) {
	var res EffectiveConfigResult

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

	if fileExists {
		res.Config = fileCfg
		res.Addr = fileCfg.Addr()
		res.DBPath = fileCfg.Server.DBPath
		res.Source = "config"
		return res, nil
	}
	res.Config = envCfg
	res.Addr = envCfg.Addr()
	res.DBPath = envCfg.Server.DBPath
	res.Source = "env"
	return res, nil
}

// extracts port integer from host:port string
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
