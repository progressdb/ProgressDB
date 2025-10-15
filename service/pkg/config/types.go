package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"gopkg.in/yaml.v3"
)

// RuntimeConfig holds runtime key sets for use by other packages.
type RuntimeConfig struct {
	BackendKeys map[string]struct{}
	SigningKeys map[string]struct{}
}

// Config is the main configuration struct.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Security  SecurityConfig  `yaml:"security"`
	Logging   LoggingConfig   `yaml:"logging"`
	Retention RetentionConfig `yaml:"retention"`
	Ingest    IngestConfig    `yaml:"ingest"`
	Telemetry TelemetryConfig `yaml:"telemetry"`
	Sensor    SensorConfig    `yaml:"sensor"`
}

// ServerConfig holds http and tls settings.
type ServerConfig struct {
	Address string    `yaml:"address"`
	Port    int       `yaml:"port"`
	DBPath  string    `yaml:"db_path"`
	TLS     TLSConfig `yaml:"tls"`
}

// TLSConfig holds TLS certificate configuration.
type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// SecurityConfig holds security related settings.
type SecurityConfig struct {
	CORS struct {
		AllowedOrigins []string `yaml:"allowed_origins"`
	} `yaml:"cors"`
	RateLimit struct {
		RPS   float64 `yaml:"rps"`
		Burst int     `yaml:"burst"`
	} `yaml:"rate_limit"`
	IPWhitelist []string `yaml:"ip_whitelist"`
	APIKeys     struct {
		Backend  []string `yaml:"backend"`
		Frontend []string `yaml:"frontend"`
		Admin    []string `yaml:"admin"`
	} `yaml:"api_keys"`
	Encryption struct {
		Use    bool     `yaml:"use"`
		Fields []string `yaml:"fields"`
	} `yaml:"encryption"`
	KMS struct {
		Endpoint      string `yaml:"endpoint"`
		DataDir       string `yaml:"data_dir"`
		Binary        string `yaml:"binary"`
		MasterKeyFile string `yaml:"master_key_file"`
		MasterKeyHex  string `yaml:"master_key_hex"`
	} `yaml:"kms"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// RetentionConfig holds configuration for the automatic purge runner.
type RetentionConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Cron         string `yaml:"cron"`
	Period       string `yaml:"period"`
	BatchSize    int    `yaml:"batch_size"`
	BatchSleepMs int    `yaml:"batch_sleep_ms"`
	DryRun       bool   `yaml:"dry_run"`
	Paused       bool   `yaml:"paused"`
	MinPeriod    string `yaml:"min_period"`
	// LockTTL controls the lease TTL used by the retention scheduler when
	// acquiring a lock to perform a run. Specified as a duration string
	// (e.g. "300s"). If zero, a sensible default will be applied.
	LockTTL Duration `yaml:"lock_ttl"`
}

// IngestConfig holds queueing and processing configuration.
type IngestConfig struct {
	Processor ProcessorConfig `yaml:"processor"`
	Queue     QueueConfig     `yaml:"queue"`
}

// ProcessorConfig controls worker concurrency and batching.
type ProcessorConfig struct {
	Workers      int `yaml:"workers"`
	MaxBatchMsgs int `yaml:"max_batch_msgs"`
	FlushMs      int `yaml:"flush_ms"`
}

// QueueConfig holds queue settings with mode selection.
type QueueConfig struct {
	Mode                 string             `yaml:"mode"` // "durable" or "memory"
	Capacity             int                `yaml:"capacity"`
	BatchSize            int                `yaml:"batch_size"`
	DrainPollInterval    Duration           `yaml:"drain_poll_interval"`
	MaxPooledBufferBytes SizeBytes          `yaml:"max_pooled_buffer_bytes"`
	Memory               MemoryQueueConfig  `yaml:"memory"`
	Durable              DurableQueueConfig `yaml:"durable"`
}

// MemoryQueueConfig holds settings for in-memory queue.
type MemoryQueueConfig struct {
	Recover          bool     `yaml:"recover"`
	TruncateInterval Duration `yaml:"truncate_interval"`
}

// DurableQueueConfig holds settings for durable queue with WAL.
type DurableQueueConfig struct {
	Recover          bool      `yaml:"recover"`
	TruncateInterval Duration  `yaml:"truncate_interval"`
	Mode             string    `yaml:"mode"` // batch | sync
	MaxFileSize      SizeBytes `yaml:"max_file_size"`
	EnableBatch      bool      `yaml:"enable_batch"`
	BatchSize        int       `yaml:"batch_size"`
	BatchInterval    Duration  `yaml:"batch_interval"`
	EnableCompress   bool      `yaml:"enable_compress"`
	CompressMinBytes int64     `yaml:"compress_min_bytes"`
	RetentionBytes   SizeBytes `yaml:"retention_bytes"`
	RetentionAge     Duration  `yaml:"retention_age"`
	// DisablePebbleWAL controls the underlying Pebble DB's WAL setting.
	// Default is true - given application level WAL
	// This is here for configuration access
	// NOTE that enabling 2 wals can decrease performance.
	DisablePebbleWAL *bool `yaml:"disable_pebble_wal"`
}

// SizeBytes represents a number of bytes, unmarshaled from human-friendly strings like "64MB" or plain integers.
type SizeBytes int64

func (s *SizeBytes) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		*s = 0
		return nil
	}
	raw := strings.TrimSpace(node.Value)
	if raw == "" {
		*s = 0
		return nil
	}
	if v, err := humanize.ParseBytes(raw); err == nil {
		*s = SizeBytes(v)
		return nil
	}
	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		*s = SizeBytes(i)
		return nil
	}
	return fmt.Errorf("invalid size value: %q", node.Value)
}

func (s SizeBytes) Int64() int64 { return int64(s) }

// Duration is a wrapper around time.Duration that supports YAML parsing from strings like "100ms" or plain numbers (interpreted as seconds).
type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		*d = Duration(0)
		return nil
	}
	raw := strings.TrimSpace(node.Value)
	if raw == "" {
		*d = Duration(0)
		return nil
	}
	if td, err := time.ParseDuration(raw); err == nil {
		*d = Duration(td)
		return nil
	}
	// allow numeric seconds
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		*d = Duration(time.Duration(f * float64(time.Second)))
		return nil
	}
	return fmt.Errorf("invalid duration value: %q", node.Value)
}

func (d Duration) Duration() time.Duration { return time.Duration(d) }

// TelemetryConfig controls sampling and slow-request thresholds.
type TelemetryConfig struct {
	SampleRate    float64  `yaml:"sample_rate"`
	SlowThreshold Duration `yaml:"slow_threshold"`
}

// SensorConfig holds sensor related tuning knobs.
type SensorConfig struct {
	Monitor struct {
		PollInterval   Duration  `yaml:"poll_interval"`
		WALHighBytes   SizeBytes `yaml:"wal_high_bytes"`
		WALLowBytes    SizeBytes `yaml:"wal_low_bytes"`
		DiskHighPct    int       `yaml:"disk_high_pct"`
		DiskLowPct     int       `yaml:"disk_low_pct"`
		RecoveryWindow Duration  `yaml:"recovery_window"`
	} `yaml:"monitor"`
}
