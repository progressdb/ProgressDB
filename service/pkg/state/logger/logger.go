package logger

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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

func Init() {
	sink := os.Getenv("PROGRESSDB_LOG_SINK")
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

	logCh = make(chan []byte, 10000)
	logStopCh = make(chan struct{})
	aw := &asyncWriter{ch: logCh}
	Log = slog.New(slog.NewTextHandler(aw, &slog.HandlerOptions{Level: level}))

	logWG.Add(1)
	go func() {
		defer logWG.Done()
		var buf *bufio.Writer
		var f *os.File
		if strings.HasPrefix(sink, "file:") {
			path := strings.TrimPrefix(sink, "file:")
			var err error
			f, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to open log file %s: %v\n", path, err)
				buf = bufio.NewWriterSize(os.Stdout, 8192)
			} else {
				buf = bufio.NewWriterSize(f, 8192)
			}
		} else {
			buf = bufio.NewWriterSize(os.Stdout, 8192)
		}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case b := <-logCh:
				buf.Write(b)
			case <-ticker.C:
				buf.Flush()
			case <-logStopCh:
				buf.Flush()
				if f != nil {
					f.Close()
				}
				return
			}
		}
	}()
}

func InitWithLevel(level string) {
	sink := os.Getenv("PROGRESSDB_LOG_SINK")
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

	logCh = make(chan []byte, 10000)
	logStopCh = make(chan struct{})
	aw := &asyncWriter{ch: logCh}
	Log = slog.New(slog.NewTextHandler(aw, &slog.HandlerOptions{Level: lv}))

	logWG.Add(1)
	go func() {
		defer logWG.Done()
		var buf *bufio.Writer
		var f *os.File
		if strings.HasPrefix(sink, "file:") {
			path := strings.TrimPrefix(sink, "file:")
			var err error
			f, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to open log file %s: %v\n", path, err)
				buf = bufio.NewWriterSize(os.Stdout, 8192)
			} else {
				buf = bufio.NewWriterSize(f, 8192)
			}
		} else {
			buf = bufio.NewWriterSize(os.Stdout, 8192)
		}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case b := <-logCh:
				buf.Write(b)
			case <-ticker.C:
				buf.Flush()
			case <-logStopCh:
				buf.Flush()
				if f != nil {
					f.Close()
				}
				return
			}
		}
	}()
}

func AttachAuditFileSink(auditDir string) error {
	if auditDir == "" {
		return fmt.Errorf("empty audit dir")
	}
	if fi, err := os.Lstat(auditDir); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("audit path is a symlink: %s", auditDir)
		}
		if !fi.IsDir() {
			return fmt.Errorf("audit path exists and is not a directory: %s", auditDir)
		}
	}
	if err := os.MkdirAll(auditDir, 0o700); err != nil {
		return fmt.Errorf("failed to create audit directory: %w", err)
	}
	if fi2, err := os.Lstat(auditDir); err == nil {
		if fi2.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("audit path is a symlink after creation: %s", auditDir)
		}
	}

	fname := filepath.Join(auditDir, "audit.log")
	if fi, err := os.Stat(fname); err == nil {
		const maxSize = 10 * 1024 * 1024
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
	Audit.Info("audit_sink_attached", "path", fname)
	return nil
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

func LogConfigSummary(title string, items []string) {
	if len(items) == 0 {
		return
	}
	human := strings.ReplaceAll(title, "_", " ")
	human = strings.Title(human)
	header := "== " + human + " "
	const width = 60
	if len(header) < width {
		header = header + strings.Repeat("=", width-len(header))
	}
	fmt.Fprintln(os.Stdout, header)
	for _, it := range items {
		fmt.Fprintln(os.Stdout, "- "+it)
	}
	fmt.Fprintln(os.Stdout)
}
