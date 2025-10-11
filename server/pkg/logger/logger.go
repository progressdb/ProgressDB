package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var Log *slog.Logger

// Audit is an optional dedicated audit logger. Callers may use
// logger.Audit.Info(...) to emit audit records; if nil, audit events
// should fall back to the main logger.
var Audit *slog.Logger

// Init initializes the global slog logger with a simple text handler at Info level.
func Init() {
	// Allow overriding sink and level via env vars for tests and production
	sink := os.Getenv("PROGRESSDB_LOG_SINK") // e.g. "file:/path/to/log"
	lvl := strings.ToLower(strings.TrimSpace(os.Getenv("PROGRESSDB_LOG_LEVEL")))
	var level slog.Level
	switch lvl {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	case "info":
		level = slog.LevelInfo
	default:
		level = slog.LevelInfo
	}

	if strings.HasPrefix(sink, "file:") {
		// write logs to file
		path := strings.TrimPrefix(sink, "file:")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err == nil {
			Log = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level}))
			return
		}
		// fallback to stdout
		fmt.Fprintf(os.Stderr, "failed to open log file %s: %v\n", path, err)
	}
	Log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

// InitWithLevel initializes the global logger but honors the provided
// `level` string ("debug", "info", "warn", "error"). If level is empty,
// InitWithLevel falls back to the environment-based behavior of Init().
func InitWithLevel(level string) {
    // Allow overriding sink and level via env vars for tests and production
    sink := os.Getenv("PROGRESSDB_LOG_SINK") // e.g. "file:/path/to/log"
    lvl := strings.ToLower(strings.TrimSpace(level))
    if lvl == "" {
        lvl = strings.ToLower(strings.TrimSpace(os.Getenv("PROGRESSDB_LOG_LEVEL")))
    }
    var lv slog.Level
    switch lvl {
    case "debug":
        lv = slog.LevelDebug
    case "warn", "warning":
        lv = slog.LevelWarn
    case "error":
        lv = slog.LevelError
    case "info":
        lv = slog.LevelInfo
    default:
        lv = slog.LevelInfo
    }

    if strings.HasPrefix(sink, "file:") {
        // write logs to file
        path := strings.TrimPrefix(sink, "file:")
        f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
        if err == nil {
            Log = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: lv}))
            return
        }
        // fallback to stdout
        fmt.Fprintf(os.Stderr, "failed to open log file %s: %v\n", path, err)
    }
    Log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}

// AttachAuditFileSink configures a simple JSON-file audit logger writing to
// <auditDir>/audit.log. If the file cannot be opened the function
// returns an error and leaves Audit as nil.
func AttachAuditFileSink(auditDir string) error {
	if auditDir == "" {
		return fmt.Errorf("empty audit dir")
	}
	// If the path exists and is a symlink, fail early to avoid TOCTOU.
	if fi, err := os.Lstat(auditDir); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("audit path is a symlink: %s", auditDir)
		}
		// If the path exists but is not a directory, fail early.
		if !fi.IsDir() {
			return fmt.Errorf("audit path exists and is not a directory: %s", auditDir)
		}
	}
	// Ensure the audit directory exists with restrictive permissions.
	if err := os.MkdirAll(auditDir, 0o700); err != nil {
		return fmt.Errorf("failed to create audit directory: %w", err)
	}
	// double-check for symlink after creation
	if fi2, err := os.Lstat(auditDir); err == nil {
		if fi2.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("audit path is a symlink after creation: %s", auditDir)
		}
	}

	// Do not enforce ownership or POSIX permission checks here. For now
	// prefer to create the audit directory and proceed; concrete handling
	// for cross-user permissions will be addressed later.
	fname := filepath.Join(auditDir, "audit.log")
	// If existing file too large, rotate it.
	if fi, err := os.Stat(fname); err == nil {
		const maxSize = 10 * 1024 * 1024 // 10MB
		if fi.Size() > maxSize {
			bak := fname + "." + fi.ModTime().UTC().Format("20060102T150405Z")
			_ = os.Rename(fname, bak)
		}
	}
	f, err := os.OpenFile(fname, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open audit log file: %w", err)
	}
	h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	Audit = slog.New(h)
	// Emit an initial marker so consumers (and tests) can observe that
	// the audit sink was successfully attached and the file is writable.
	Audit.Info("audit_sink_attached", "path", fname)
	return nil
}

// Sync is a no-op for slog handlers used here.
func Sync() {}

// Debug logs with slog-style key/value pairs.
func Debug(msg string, args ...any) {
	if Log == nil {
		return
	}
	Log.Debug(msg, args...)
}

// Info logs with slog-style key/value pairs.
func Info(msg string, args ...any) {
	if Log == nil {
		return
	}
	Log.Info(msg, args...)
}

// Warn logs with slog-style key/value pairs.
func Warn(msg string, args ...any) {
	if Log == nil {
		return
	}
	Log.Warn(msg, args...)
}

// Error logs with slog-style key/value pairs.
func Error(msg string, args ...any) {
	if Log == nil {
		return
	}
	Log.Error(msg, args...)
}
