package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var Log *slog.Logger
var Audit *slog.Logger

type asyncWriter struct {
	ch chan []byte
}

func (a *asyncWriter) Write(p []byte) (n int, err error) {
	cp := make([]byte, len(p))
	copy(cp, p)
	select {
	case a.ch <- cp:
		return len(p), nil
	default:
		return len(p), nil
	}
}

var logCh chan []byte
var logStopCh chan struct{}
var logWG sync.WaitGroup

func Init(level string, dbPath string) {
	var slogLevel slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	case "info":
		slogLevel = slog.LevelInfo
	default:
		slogLevel = slog.LevelInfo
	}

	// Simple console logger - no files, no async complications
	Log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel}))

	// Initialize audit logger (simplified)
	// attachAuditLogger(filepath.Join(dbPath, "state", "logs"))
}

func attachAuditLogger(logsDir string) {
	fname := filepath.Join(logsDir, "audit.log")
	if fi, err := os.Stat(fname); err == nil {
		const maxSize = 10 * 1024 * 1024
		if fi.Size() > maxSize {
			bak := fname + "." + fi.ModTime().UTC().Format("20060102T150405Z")
			_ = os.Rename(fname, bak)
		}
	}
	f, err := os.OpenFile(fname, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open audit log file: %v\n", err)
		return
	}
	h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	Audit = slog.New(h)
	Audit.Info("audit_sink_attached", "path", fname)
}

func Sync() {
	if logStopCh != nil {
		close(logStopCh)
		logWG.Wait()
	}
}

func Debug(msg string, args ...any) {
	if Log == nil {
		return
	}
	Log.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	if Log == nil {
		return
	}
	Log.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	if Log == nil {
		return
	}
	Log.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	if Log == nil {
		return
	}
	Log.Error(msg, args...)
}
