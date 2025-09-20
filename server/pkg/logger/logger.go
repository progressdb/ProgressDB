package logger

import (
	"fmt"
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

// convertMapToFields converts a generic map into zap fields.
func convertMapToFields(m map[string]interface{}) []zap.Field {
	if len(m) == 0 {
		return nil
	}
	out := make([]zap.Field, 0, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			out = append(out, zap.String(k, val))
		case bool:
			out = append(out, zap.Bool(k, val))
		case int:
			out = append(out, zap.Int(k, val))
		case int8:
			out = append(out, zap.Int8(k, val))
		case int16:
			out = append(out, zap.Int16(k, val))
		case int32:
			out = append(out, zap.Int32(k, val))
		case int64:
			out = append(out, zap.Int64(k, val))
		case uint:
			out = append(out, zap.Uint(k, val))
		case uint8:
			out = append(out, zap.Uint8(k, val))
		case uint16:
			out = append(out, zap.Uint16(k, val))
		case uint32:
			out = append(out, zap.Uint32(k, val))
		case uint64:
			out = append(out, zap.Uint64(k, val))
		case float32:
			out = append(out, zap.Float32(k, val))
		case float64:
			out = append(out, zap.Float64(k, val))
		case error:
			out = append(out, zap.Error(val))
		case []byte:
			out = append(out, zap.ByteString(k, val))
		default:
			out = append(out, zap.Any(k, val))
		}
	}
	return out
}

// High-level logging helpers that accept a message name and a map of key->value.
// Callers can pass nil or an empty map when no structured fields are needed.
func Debug(msg string, fields map[string]interface{}) {
	if Log == nil {
		return
	}
	Log.Debug(msg, convertMapToFields(fields)...)
}

func Info(msg string, fields map[string]interface{}) {
	if Log == nil {
		return
	}
	Log.Info(msg, convertMapToFields(fields)...)
}

func Warn(msg string, fields map[string]interface{}) {
	if Log == nil {
		return
	}
	Log.Warn(msg, convertMapToFields(fields)...)
}

func Error(msg string, fields map[string]interface{}) {
	if Log == nil {
		return
	}
	Log.Error(msg, convertMapToFields(fields)...)
}

func DPanic(msg string, fields map[string]interface{}) {
	if Log == nil {
		return
	}
	Log.DPanic(msg, convertMapToFields(fields)...)
}

func Panic(msg string, fields map[string]interface{}) {
	if Log == nil {
		return
	}
	Log.Panic(msg, convertMapToFields(fields)...)
}

func Fatal(msg string, fields map[string]interface{}) {
	if Log == nil {
		return
	}
	Log.Fatal(msg, convertMapToFields(fields)...)
}

// kvToFields converts variadic key/value pairs into zap fields. Keys are
// coerced to strings using fmt.Sprint when not a string. If an odd number of
// elements is provided, the last key will have a nil value.
func kvToFields(kv []interface{}) []zap.Field {
	if len(kv) == 0 {
		return nil
	}
	out := make([]zap.Field, 0, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		var key string
		if s, ok := kv[i].(string); ok {
			key = s
		} else {
			key = fmt.Sprint(kv[i])
		}
		var v interface{}
		if i+1 < len(kv) {
			v = kv[i+1]
		} else {
			v = nil
		}
		switch val := v.(type) {
		case string:
			out = append(out, zap.String(key, val))
		case bool:
			out = append(out, zap.Bool(key, val))
		case int:
			out = append(out, zap.Int(key, val))
		case int8:
			out = append(out, zap.Int8(key, val))
		case int16:
			out = append(out, zap.Int16(key, val))
		case int32:
			out = append(out, zap.Int32(key, val))
		case int64:
			out = append(out, zap.Int64(key, val))
		case uint:
			out = append(out, zap.Uint(key, val))
		case uint8:
			out = append(out, zap.Uint8(key, val))
		case uint16:
			out = append(out, zap.Uint16(key, val))
		case uint32:
			out = append(out, zap.Uint32(key, val))
		case uint64:
			out = append(out, zap.Uint64(key, val))
		case float32:
			out = append(out, zap.Float32(key, val))
		case float64:
			out = append(out, zap.Float64(key, val))
		case error:
			out = append(out, zap.NamedError(key, val))
		case []byte:
			out = append(out, zap.ByteString(key, val))
		default:
			out = append(out, zap.Any(key, val))
		}
	}
	return out
}

// Variadic KV helpers
func DebugKV(msg string, kv ...interface{}) {
	if Log == nil {
		return
	}
	Log.Debug(msg, kvToFields(kv)...)
}
func InfoKV(msg string, kv ...interface{}) {
	if Log == nil {
		return
	}
	Log.Info(msg, kvToFields(kv)...)
}
func WarnKV(msg string, kv ...interface{}) {
	if Log == nil {
		return
	}
	Log.Warn(msg, kvToFields(kv)...)
}
func ErrorKV(msg string, kv ...interface{}) {
	if Log == nil {
		return
	}
	Log.Error(msg, kvToFields(kv)...)
}
func DPanicKV(msg string, kv ...interface{}) {
	if Log == nil {
		return
	}
	Log.DPanic(msg, kvToFields(kv)...)
}
func PanicKV(msg string, kv ...interface{}) {
	if Log == nil {
		return
	}
	Log.Panic(msg, kvToFields(kv)...)
}
func FatalKV(msg string, kv ...interface{}) {
	if Log == nil {
		return
	}
	Log.Fatal(msg, kvToFields(kv)...)
}
