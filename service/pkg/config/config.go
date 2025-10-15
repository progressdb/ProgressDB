package config

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/adhocore/gronx"
	"gopkg.in/yaml.v3"
)

// Defaults and limits for queue/WAL configuration
const (
	defaultQueueCapacity    = 1048576                // 1M as per new config
	defaultWALMaxFileSize   = 2 * 1024 * 1024 * 1024 // 2 GiB
	defaultWALBatchInterval = 10 * time.Millisecond
	defaultWALBatchSize     = 4096
	minWALFileSize          = 1 * 1024 * 1024 // 1 MiB
	minWALBatchInterval     = 1 * time.Millisecond
	defaultCompressMinBytes = 512
	// Ingest/processor defaults
	defaultProcessorWorkers      = 48
	defaultProcessorMaxBatchMsgs = 10000
	defaultProcessorFlushMs      = 1

	// Queue defaults
	defaultQueueBatchSize        = 131072
	defaultDrainPollInterval     = 250 * time.Microsecond
	defaultMaxPooledBufferBytes  = 3 * 1024 * 1024 * 1024 // 3 GiB
	defaultQueueTruncateInterval = 60 * time.Second
	// Retention defaults
	defaultRetentionLockTTL = 300 * time.Second
	defaultRetentionCron    = "0 2 * * *" // Default to daily at 02:00
	// telemetry defaults
	defaultTelemetrySampleRate = 0.001
	defaultTelemetrySlowMs     = 200
	// sensor defaults
	defaultSensorPollInterval   = 500 * time.Millisecond
	defaultSensorWALHighBytes   = 1 << 30 // 1 GiB
	defaultSensorWALLowBytes    = 700 << 20
	defaultSensorDiskHighPct    = 80
	defaultSensorDiskLowPct     = 60
	defaultSensorRecoveryWindow = 5 * time.Second
)

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

// Validate applies defaults and validates values in the config. It mutates
// the receiver to fill in missing defaults and returns an error if any
// configuration value is invalid.
func (c *Config) ValidateConfig() error {
	// Queue defaults
	if c.Ingest.Queue.Capacity <= 0 {
		c.Ingest.Queue.Capacity = defaultQueueCapacity
	}
	// Queue batch size (used by batch consumers)
	if c.Ingest.Queue.BatchSize <= 0 {
		c.Ingest.Queue.BatchSize = defaultQueueBatchSize
	}
	// Drain poll interval (used when closing/draining the queue)
	if c.Ingest.Queue.DrainPollInterval.Duration() == 0 {
		c.Ingest.Queue.DrainPollInterval = Duration(defaultDrainPollInterval)
	}
	// max pooled buffer size
	if c.Ingest.Queue.MaxPooledBufferBytes.Int64() == 0 {
		c.Ingest.Queue.MaxPooledBufferBytes = SizeBytes(defaultMaxPooledBufferBytes)
	}
	// WAL defaults and validation
	wc := &c.Ingest.Queue.Durable
	if wc.MaxFileSize.Int64() == 0 {
		wc.MaxFileSize = SizeBytes(defaultWALMaxFileSize)
	}
	if wc.MaxFileSize.Int64() < minWALFileSize {
		return fmt.Errorf("durable.max_file_size must be >= %d bytes", minWALFileSize)
	}
	if wc.BatchInterval.Duration() == 0 {
		wc.BatchInterval = Duration(defaultWALBatchInterval)
	}
	if wc.BatchInterval.Duration() < minWALBatchInterval {
		return fmt.Errorf("durable.batch_interval must be >= %s", minWALBatchInterval)
	}
	if wc.BatchSize <= 0 {
		wc.BatchSize = defaultWALBatchSize
	}
	if wc.CompressMinBytes == 0 {
		wc.CompressMinBytes = defaultCompressMinBytes
	}

	// Default is to disable Pebble WAL.
	if wc.DisablePebbleWAL == nil {
		def := true
		wc.DisablePebbleWAL = &def
	}

	// Normalize mode
	switch wc.Mode {
	case "", "batch", "sync", "none":
		// ok
	default:
		return fmt.Errorf("durable.mode must be one of: none, batch, sync")
	}

	// Processor defaults
	pc := &c.Ingest.Processor
	if pc.Workers <= 0 {
		pc.Workers = runtime.NumCPU()
	}
	if pc.MaxBatchMsgs <= 0 {
		pc.MaxBatchMsgs = defaultProcessorMaxBatchMsgs
	}
	if pc.FlushMs <= 0 {
		pc.FlushMs = defaultProcessorFlushMs
	}

	// Telemetry defaults
	if c.Telemetry.SampleRate == 0 {
		c.Telemetry.SampleRate = defaultTelemetrySampleRate
	}
	if c.Telemetry.SlowThreshold.Duration() == 0 {
		c.Telemetry.SlowThreshold = Duration(time.Duration(defaultTelemetrySlowMs) * time.Millisecond)
	}

	// Security defaults: rate limiting
	if c.Security.RateLimit.RPS <= 0 {
		c.Security.RateLimit.RPS = 1000
	}
	if c.Security.RateLimit.Burst <= 0 {
		c.Security.RateLimit.Burst = 1000
	}

	// Sensor monitor defaults
	if c.Sensor.Monitor.PollInterval.Duration() == 0 {
		c.Sensor.Monitor.PollInterval = Duration(defaultSensorPollInterval)
	}
	if c.Sensor.Monitor.WALHighBytes.Int64() == 0 {
		c.Sensor.Monitor.WALHighBytes = SizeBytes(defaultSensorWALHighBytes)
	}
	if c.Sensor.Monitor.WALLowBytes.Int64() == 0 {
		c.Sensor.Monitor.WALLowBytes = SizeBytes(defaultSensorWALLowBytes)
	}
	if c.Sensor.Monitor.DiskHighPct == 0 {
		c.Sensor.Monitor.DiskHighPct = defaultSensorDiskHighPct
	}
	if c.Sensor.Monitor.DiskLowPct == 0 {
		c.Sensor.Monitor.DiskLowPct = defaultSensorDiskLowPct
	}
	if c.Sensor.Monitor.RecoveryWindow.Duration() == 0 {
		c.Sensor.Monitor.RecoveryWindow = Duration(defaultSensorRecoveryWindow)
	}

	// Retention defaults
	// Retention lock TTL
	if c.Retention.LockTTL.Duration() == 0 {
		c.Retention.LockTTL = Duration(defaultRetentionLockTTL)
	}
	// Retention cron (if not set, default to daily at 02:00)
	if c.Retention.Cron == "" {
		c.Retention.Cron = defaultRetentionCron
	}

	// Validate user-passed retention cron for correctness.
	// Only validate if the cron string is set (either set by user, or by the default above).
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
