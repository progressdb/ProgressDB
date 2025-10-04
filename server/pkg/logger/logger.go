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
        // check perms: disallow group/other write to avoid insecure dirs
        if fi.Mode().Perm()&0o022 != 0 {
            return fmt.Errorf("audit directory has permissive mode (group/other write): %s", auditDir)
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
        if fi2.Mode().Perm()&0o022 != 0 {
            return fmt.Errorf("audit directory has permissive mode after creation: %s", auditDir)
        }
    }
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
