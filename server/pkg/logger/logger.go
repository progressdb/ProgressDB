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

// AttachAuditFileSink configures a simple JSON-file audit logger writing to
// auditDir/<YEAR>/audit.log. If the file cannot be opened the function
// returns an error and leaves Audit as nil.
func AttachAuditFileSink(auditDir string) error {
    if auditDir == "" {
        return fmt.Errorf("empty audit dir")
    }
    // Ensure the audit directory exists with restrictive permissions.
    // Try the provided path first; if that fails we'll climb one step up
    // and place audit logs under <parent>/logs/audit.log to avoid writing
    // into the DB path when that is not writable.
    tryCreate := func(dir string) (string, error) {
        if err := os.MkdirAll(dir, 0o700); err != nil {
            return "", err
        }
        fname := filepath.Join(dir, "audit.log")
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
            return "", err
        }
        // wrap and return
        h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
        Audit = slog.New(h)
        return fname, nil
    }

    if _, err := tryCreate(auditDir); err == nil {
        return nil
    }

    // fallback: climb one level and use <parent>/logs/audit.log
    parent := filepath.Dir(auditDir)
    altDir := filepath.Join(parent, "logs")
    if _, err := tryCreate(altDir); err == nil {
        return nil
    }

    return fmt.Errorf("failed to create audit sink at %s or %s", auditDir, altDir)
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
