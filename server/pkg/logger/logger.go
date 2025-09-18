package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.Logger

// Init initializes the global Zap logger. Mode can be "dev" or "prod".
// Environment overrides:
// - PROGRESSDB_LOG_MODE: dev|prod (default: dev)
// - PROGRESSDB_LOG_SINK: stdout (default) or file:/path/to/file
// - PROGRESSDB_LOG_LEVEL: debug|info|warn|error (default: info for prod, debug for dev)
func Init() error {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("PROGRESSDB_LOG_MODE")))
	if mode == "" {
		mode = "dev"
	}

	sink := strings.TrimSpace(os.Getenv("PROGRESSDB_LOG_SINK"))
	if sink == "" {
		sink = "stdout"
	}

	lvlStr := strings.ToLower(strings.TrimSpace(os.Getenv("PROGRESSDB_LOG_LEVEL")))

	var cfg zap.Config
	if mode == "prod" || mode == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		// dev defaults to console encoder
	}

	// Keep output concise: disable caller and stacktrace by default so logs
	// are not noisy. Time format set to RFC3339.
	cfg.DisableCaller = true
	cfg.DisableStacktrace = true
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder

	// Set level if provided
	if lvlStr != "" {
		var lvl zapcore.Level
		switch lvlStr {
		case "debug":
			lvl = zapcore.DebugLevel
		case "info":
			lvl = zapcore.InfoLevel
		case "warn", "warning":
			lvl = zapcore.WarnLevel
		case "error":
			lvl = zapcore.ErrorLevel
		default:
			lvl = cfg.Level.Level()
		}
		cfg.Level = zap.NewAtomicLevelAt(lvl)
	}

	// Configure output sink
	if strings.HasPrefix(sink, "file:") {
		path := strings.TrimPrefix(sink, "file:")
		if path == "" {
			cfg.OutputPaths = []string{"stdout"}
		} else {
			cfg.OutputPaths = []string{path}
		}
	} else {
		cfg.OutputPaths = []string{"stdout"}
	}

	l, err := cfg.Build()
	if err != nil {
		return err
	}
	Log = l
	return nil
}

// Sync flushes buffered logs.
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
