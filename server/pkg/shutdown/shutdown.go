package shutdown

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"progressdb/pkg/logger"
	"runtime"
	"syscall"
	"time"
)

type exitRequest struct {
	Time      string            `json:"time"`
	Reason    string            `json:"reason"`
	Cmd       string            `json:"cmd"`
	CrashPath string            `json:"crash_path,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

// Controlled abord handling from other parts of the code
func Abort(contextMsg string, err error, dbPath string, delaySeconds ...int) {
    // default abort delay (seconds). Keep at 10s so crash dumps and logs
    // have time to flush before exit.
    delay := 10
	if len(delaySeconds) > 0 && delaySeconds[0] >= 0 {
		delay = delaySeconds[0]
	}
	logger.Error("startup_fatal", "msg", contextMsg, "error", err)
	dumpPath, reqPath, derr := AbortWithDiagnostics(dbPath, contextMsg, err)
	if derr != nil {
		logger.Error("abort_with_diagnostics_failed", "error", derr)
		fmt.Fprintf(os.Stderr, "FAILED TO WRITE CRASH DUMP: %v\n", derr)
	} else {
		logger.Info("wrote_crash_dump", "path", dumpPath, "request", reqPath)
		logger.Error("startup_fatal_crashdump", "path", dumpPath)
		fmt.Fprintf(os.Stderr, "CRASH DUMP WRITTEN: %s\n", dumpPath)
	}
	for i := delay; i > 0; i-- {
		logger.Info("exiting_in_seconds", "seconds", i)
		time.Sleep(1 * time.Second)
	}
	os.Exit(2)
}

// AbortWithDiagnostics writes a crash dump and a shutdown request file
func AbortWithDiagnostics(dbPath, reason string, err error) (string, string, error) {
	// crash dir (large human-readable dumps) and abort dir (machine-readable
	// exit/abort requests). When a crash happens we write both: a crash dump
	// into crashDir and an abort request into abortDir that references the
	// crash path.
	crashDir := "./crash"
	abortDir := "./abort"
	if dbPath != "" {
		crashDir = filepath.Join(dbPath, "state", "crash")
		abortDir = filepath.Join(dbPath, "state", "abort")
	}
	if e := os.MkdirAll(crashDir, 0o700); e != nil {
		return "", "", fmt.Errorf("failed to create crash dir: %w", e)
	}
	if e := os.MkdirAll(abortDir, 0o700); e != nil {
		return "", "", fmt.Errorf("failed to create abort dir: %w", e)
	}

	ts := time.Now().UnixNano()
	dumpName := fmt.Sprintf("crash-%d.log", ts)
	dumpPath := filepath.Join(crashDir, dumpName)

	// build dump
	f, ferr := os.CreateTemp(crashDir, ".crash-*.tmp")
	if ferr != nil {
		return "", "", fmt.Errorf("failed to create temp crash file: %w", ferr)
	}
	// ensure temp removed if we fail
	tmpName := f.Name()
	defer func() { _ = os.Remove(tmpName) }()

	fmt.Fprintf(f, "time: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(f, "reason: %s\n", reason)
	fmt.Fprintf(f, "error: %v\n", err)
	fmt.Fprintf(f, "\n--- environ ---\n")
	for _, e := range os.Environ() {
		fmt.Fprintln(f, e)
	}
	fmt.Fprintf(f, "\n--- goroutine stacks ---\n")
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	f.Write(buf[:n])
	f.Sync()
	f.Close()

	if err := os.Rename(tmpName, dumpPath); err != nil {
		return "", "", fmt.Errorf("failed to move crash dump into place: %w", err)
	}
	_ = os.Chmod(dumpPath, 0o600)

	// create abort request referencing the crash dump
	req := exitRequest{
		Time:      time.Now().UTC().Format(time.RFC3339),
		Reason:    reason,
		Cmd:       "crash",
		CrashPath: dumpPath,
		Meta:      map[string]string{"pid": fmt.Sprintf("%d", os.Getpid())},
	}
	rtmp, rerr := os.CreateTemp(abortDir, ".req-*.tmp")
	if rerr != nil {
		return dumpPath, "", fmt.Errorf("failed to create temp req file: %w", rerr)
	}
	rname := rtmp.Name()
	enc := json.NewEncoder(rtmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(req); err != nil {
		rtmp.Close()
		_ = os.Remove(rname)
		return dumpPath, "", fmt.Errorf("failed to encode req: %w", err)
	}
	rtmp.Sync()
	rtmp.Close()

	reqName := fmt.Sprintf("req-%d.json", ts)
	reqPath := filepath.Join(abortDir, reqName)
	if err := os.Rename(rname, reqPath); err != nil {
		_ = os.Remove(rname)
		return dumpPath, "", fmt.Errorf("failed to move req into place: %w", err)
	}
	_ = os.Chmod(reqPath, 0o600)

	return dumpPath, reqPath, nil
}

// RequestExitFile writes a simple exit request (no dump) and returns its path.
func RequestExitFile(dbPath, reason string) (string, error) {
	// write an operator-requested abort file (no crash dump)
	abortDir := "./abort"
	if dbPath != "" {
		abortDir = filepath.Join(dbPath, "state", "abort")
	}
	if err := os.MkdirAll(abortDir, 0o700); err != nil {
		return "", err
	}
	ts := time.Now().UnixNano()
	req := exitRequest{Time: time.Now().UTC().Format(time.RFC3339), Reason: reason, Cmd: "abort", Meta: map[string]string{"pid": fmt.Sprintf("%d", os.Getpid())}}
	rtmp, err := os.CreateTemp(abortDir, ".req-*.tmp")
	if err != nil {
		return "", err
	}
	name := rtmp.Name()
	enc := json.NewEncoder(rtmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(req); err != nil {
		rtmp.Close()
		_ = os.Remove(name)
		return "", err
	}
	rtmp.Sync()
	rtmp.Close()
	reqName := fmt.Sprintf("req-%d.json", ts)
	reqPath := filepath.Join(abortDir, reqName)
	if err := os.Rename(name, reqPath); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	_ = os.Chmod(reqPath, 0o600)
	return reqPath, nil
}

// SetupSignalHandler installs handlers for SIGINT/SIGTERM and SIGPIPE and
// returns a cancellable context. The returned context is cancelled when any
// of the watched signals arrives. Use the cancel function to stop watching
// and to release resources.
func SetupSignalHandler(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	// handle interrupt/terminate for graceful shutdown
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigc
		logger.Info("signal_received", "signal", s.String(), "msg", "shutdown requested")
		cancel()
	}()

	// watch for SIGPIPE and dump goroutine stacks to aid diagnostics
	sigpipe := make(chan os.Signal, 1)
	signal.Notify(sigpipe, syscall.SIGPIPE)
	go func() {
		s := <-sigpipe
		logger.Info("signal_received", "signal", s.String(), "msg", "SIGPIPE - dumping goroutine stacks")
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		logger.Info("goroutine_stack_dump", "dump", string(buf[:n]))
		cancel()
	}()

	return ctx, cancel
}
