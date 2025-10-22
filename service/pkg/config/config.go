package config

import (
	"fmt"
	"os"
	"runtime"
	"sync"

	"progressdb/pkg/logger"

	"github.com/adhocore/gronx"
	"github.com/goccy/go-yaml"
)

// Defaults and limits for queue/WAL configuration
// const (
// 	defaultQueueCapacity    = 4 * 1024 * 1024        // 4M for higher buffer
// 	defaultWALMaxFileSize   = 2 * 1024 * 1024 * 1024 // 2 GiB
// 	defaultWALBatchInterval = 10 * time.Millisecond
// 	defaultWALBatchSize     = 4096
// 	minWALFileSize          = 1 * 1024 * 1024 // 1 MiB
// 	minWALBatchInterval     = 1 * time.Millisecond
// 	defaultCompressMinBytes = 512
// 	// Ingest/ingestor defaults
// 	defaultIngestorWorkerCount          = 48
// 	defaultIngestorApplyQueueBufferSize = 100
// 	defaultIngestorMaxBatchSize         = 10000
// 	defaultIngestorFlushIntervalMs      = 1

// 	// Queue defaults
// 	defaultQueueBatchSize        = 131072
// 	defaultDrainPollInterval     = 250 * time.Microsecond
// 	defaultMaxPooledBufferBytes  = 3 * 1024 * 1024 * 1024 // 3 GiB
// 	defaultQueueTruncateInterval = 60 * time.Second
// 	// Retention defaults
// 	defaultRetentionLockTTL = 300 * time.Second
// 	defaultRetentionCron    = "0 2 * * *" // Default to daily at 02:00
// 	// telemetry defaults
// 	defaultTelemetrySampleRate    = 0.001
// 	defaultTelemetrySlowMs        = 200
// 	defaultTelemetryBufferSize    = 60 * 1024 * 1024 // 60MB
// 	defaultTelemetryFileMaxSize   = 40 * 1024 * 1024 // 40MB
// 	defaultTelemetryFlushMs       = 2000             // 2 seconds
// 	defaultTelemetryQueueCapacity = 2048
// 	// sensor defaults
// 	defaultSensorPollInterval   = 500 * time.Millisecond
// 	defaultSensorDiskHighPct    = 80
// 	defaultSensorDiskLowPct     = 60
// 	defaultSensorMemHighPct     = 80
// 	defaultSensorCPUHighPct     = 90
// 	defaultSensorRecoveryWindow = 5 * time.Second
// )

var (
	runtimeMu  sync.RWMutex
	runtimeCfg *RuntimeConfig
)

// SetRuntime sets the global runtime config.
func SetRuntime(rc *RuntimeConfig) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	runtimeCfg = rc
}

// GetBackendKeys returns a copy of backend API keys.
func GetBackendKeys() map[string]struct{} {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	out := make(map[string]struct{})
	if runtimeCfg == nil || runtimeCfg.BackendKeys == nil {
		return out
	}
	for k := range runtimeCfg.BackendKeys {
		out[k] = struct{}{}
	}
	return out
}

// GetSigningKeys returns a copy of signing keys.
func GetSigningKeys() map[string]struct{} {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	out := make(map[string]struct{})
	if runtimeCfg == nil || runtimeCfg.SigningKeys == nil {
		return out
	}
	for k := range runtimeCfg.SigningKeys {
		out[k] = struct{}{}
	}
	return out
}

// Addr returns the HTTP server address as host:port.
func (c *Config) Addr() string {
	addr := c.Server.Address
	if addr == "" {
		addr = "0.0.0.0"
	}
	port := c.Server.Port
	if port == 0 {
		port = 8080
	}
	return fmt.Sprintf("%s:%d", addr, port)
}

// LoadConfigFile reads and parses a config file.
func LoadConfigFile(path string) (*Config, error) {
	b, err := os.ReadFile(path)
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

// Validate validates values in the config and applies any runtime-specific logic.
func (c *Config) ValidateConfig() error {
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	logger.Info("system_logical_cores", "logical_cores", numCPU)
	cc := &c.Ingest.Compute
	if cc.WorkerCount > numCPU {
		logger.Warn("worker_count_capped", "requested", cc.WorkerCount, "capped_to", numCPU)
		cc.WorkerCount = numCPU
	}

	// Validate user-passed retention cron for correctness.
	if !gronx.IsValid(c.Retention.Cron) {
		return fmt.Errorf("invalid retention cron expression: %s", c.Retention.Cron)
	}

	return nil
}

// ResolveConfigPath returns the config file path, preferring flag, then env.
func ResolveConfigPath(flagPath string, flagSet bool) string {
	if flagSet {
		return flagPath
	}
	if p := os.Getenv("PROGRESSDB_SERVER_CONFIG"); p != "" {
		return p
	}
	if p := os.Getenv("PROGRESSDB_CONFIG"); p != "" {
		return p
	}
	return flagPath
}
