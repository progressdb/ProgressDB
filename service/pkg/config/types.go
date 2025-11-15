package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/goccy/go-yaml/ast"
)

// RuntimeConfig holds runtime key sets for use by other packages.
type RuntimeConfig struct {
	BackendKeys    map[string]struct{}
	SigningKeys    map[string]struct{}
	MaxPayloadSize int64
}

// Config is the main configuration struct.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Storage    StorageConfig    `yaml:"storage"`
	Logging    LoggingConfig    `yaml:"logging"`
	Retention  RetentionConfig  `yaml:"retention"`
	Ingest     IngestConfig     `yaml:"ingest"`
	Telemetry  TelemetryConfig  `yaml:"telemetry"`
	Sensor     SensorConfig     `yaml:"sensor"`
	Encryption EncryptionConfig `yaml:"encryption"`
}

// ServerConfig holds http and security settings.
type ServerConfig struct {
	Address        string       `yaml:"address,default=0.0.0.0"`
	Port           int          `yaml:"port,default=8080"`
	DBPath         string       `yaml:"db_path,default=./database"`
	MaxPayloadSize SizeBytes    `yaml:"max_payload_size,default=100KB"`
	CORS           CORSConfig   `yaml:"cors"`
	RateLimit      RateConfig   `yaml:"rate_limit"`
	IPWhitelist    []string     `yaml:"ip_whitelist"`
	APIKeys        APIKeyConfig `yaml:"api_keys"`
}

// StorageConfig holds database-specific settings.
type StorageConfig struct {
}

// CORSConfig holds CORS settings.
type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

// RateConfig holds rate limiting settings.
type RateConfig struct {
	RPS   float64 `yaml:"rps,default=1000"`
	Burst int     `yaml:"burst,default=1000"`
}

// APIKeyConfig holds API key settings.
type APIKeyConfig struct {
	Backend  []string `yaml:"backend"`
	Frontend []string `yaml:"frontend"`
	Admin    []string `yaml:"admin"`
	Signing  []string `yaml:"signing"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level string `yaml:"level,default=info"`
}

// RetentionConfig holds configuration for the automatic purge runner.
type RetentionConfig struct {
	Enabled bool          `yaml:"enabled,default=false"`
	Cron    string        `yaml:"cron,default=0 2 * * *"` // Default to daily at 02:00
	MTTL    time.Duration `yaml:"mttl,default=720h"`      // Message TTL after delete (30 days)
	TTTL    time.Duration `yaml:"tttl,default=720h"`      // Thread TTL after delete (30 days)
}

// IngestConfig holds intake, compute, and apply configuration.
type IngestConfig struct {
	Intake  IntakeConfig  `yaml:"intake"`
	Compute ComputeConfig `yaml:"compute"`
	Apply   ApplyConfig   `yaml:"apply"`
}

// IntakeConfig controls enqueue buffering and persistence.
type IntakeConfig struct {
	QueueCapacity        int            `yaml:"queue_capacity,default=4194304"` // 4M for higher buffer
	ShutdownPollInterval Duration       `yaml:"shutdown_poll_interval,default=250µs"`
	WAL                  WALConfig      `yaml:"wal"`
	Recovery             RecoveryConfig `yaml:"recovery"`
}

// ComputeConfig controls worker concurrency for mutation processing.
type ComputeConfig struct {
	WorkerMultiplier int `yaml:"worker_multiplier,default=4"` // Calculate workers as multiplier x CPU cores
	WorkerCount      int `yaml:"-"`                           // Calculated field: WorkerMultiplier x CPU cores
	BufferCapacity   int `yaml:"buffer_capacity,default=1000"`
}

// ApplyConfig controls batching and queuing for DB applies.
type ApplyConfig struct {
	BatchCount int `yaml:"batch_count,default=10000"`

	BatchTimeout Duration `yaml:"batch_timeout,default=1s"`
}

// QueueConfig holds queue settings.
type QueueConfig struct {
	BufferCapacity       int       `yaml:"buffer_capacity"`
	ShutdownPollInterval Duration  `yaml:"shutdown_poll_interval"`
	WAL                  WALConfig `yaml:"wal"`
}

// WALConfig holds settings for WAL backup.
type WALConfig struct {
	Enabled     bool      `yaml:"enabled,default=false"`
	SegmentSize SizeBytes `yaml:"segment_size,default=2GB"`
}

// RecoveryConfig controls crash recovery behavior.
type RecoveryConfig struct {
	Enabled        bool `yaml:"enabled,default=true"`
	TempIdxEnabled bool `yaml:"temp_index_enabled,default=true"`
}

// SizeBytes represents a number of bytes, unmarshaled from human-friendly strings like "64MB" or plain integers.
type SizeBytes int64

func (s *SizeBytes) UnmarshalYAML(node ast.Node) error {
	if node == nil {
		*s = 0
		return nil
	}
	stringNode, ok := node.(*ast.StringNode)
	if !ok {
		return fmt.Errorf("expected string node for SizeBytes, got %T", node)
	}
	raw := strings.TrimSpace(stringNode.Value)
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
	return fmt.Errorf("invalid size value: %q", stringNode.Value)
}

func (s SizeBytes) Int64() int64 { return int64(s) }

// Duration is a wrapper around time.Duration that supports YAML parsing from strings like "100ms" or plain numbers (interpreted as seconds).
type Duration time.Duration

func (d *Duration) UnmarshalYAML(node ast.Node) error {
	if node == nil {
		*d = Duration(0)
		return nil
	}
	stringNode, ok := node.(*ast.StringNode)
	if !ok {
		return fmt.Errorf("expected string node for Duration, got %T", node)
	}
	raw := strings.TrimSpace(stringNode.Value)
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
	return fmt.Errorf("invalid duration value: %q", stringNode.Value)
}

func (d Duration) Duration() time.Duration { return time.Duration(d) }

// TelemetryConfig controls telemetry collection and storage settings.
type TelemetryConfig struct {
	BufferSize    SizeBytes `yaml:"buffer_size,default=60MB"`
	FileMaxSize   SizeBytes `yaml:"file_max_size,default=40MB"`
	FlushInterval Duration  `yaml:"flush_interval,default=2s"`
	QueueCapacity int       `yaml:"queue_capacity,default=2048"`
}

// SensorConfig holds sensor related tuning knobs.
type SensorConfig struct {
	PollInterval   Duration `yaml:"poll_interval,default=500ms"`
	DiskHighPct    int      `yaml:"disk_high_pct,default=80"`
	MemHighPct     int      `yaml:"mem_high_pct,default=80"`
	CPUHighPct     int      `yaml:"cpu_high_pct,default=90"`
	RecoveryWindow Duration `yaml:"recovery_window,default=5s"`
}

// EncryptionConfig holds encryption related settings.
type EncryptionConfig struct {
	Enabled bool     `yaml:"enabled,default=false"`
	Fields  []string `yaml:"fields"`
	KMS     struct {
		Mode          string `yaml:"mode,default=embedded"`
		Endpoint      string `yaml:"endpoint,default=127.0.0.1:6820"`
		DBPath        string `yaml:"db_path,default=./kms"`
		MasterKeyFile string `yaml:"master_key_file"`
		MasterKeyHex  string `yaml:"master_key_hex"`
	} `yaml:"kms"`
}
