package utils

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Options for test server process.
type ServerOpts struct {
	// If non-empty, write to config file, else generate minimal config.
	ConfigYAML string
	// 0 picks a free port.
	Port int
	// Extra environment variables.
	Env map[string]string
	// Optional binary to run, empty triggers build.
	BinaryPath string
}

// Running server process and paths.
type ServerProcess struct {
	Addr       string // http://host:port
	Cmd        *exec.Cmd
	StdoutPath string
	StderrPath string
	ConfigPath string
	WorkDir    string
	exitCh     chan error
}

// Copies src to dst (overwrites dst).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// Attempt to build progressdb binary at outPath.
// Tries module root (via 'go env GOMOD') and other candidate dirs.
func buildProgressdbBin(outPath string) error {
	// Try module root.
	gomodCmd := exec.Command("go", "env", "GOMOD")
	gomodCmd.Env = os.Environ()
	if b, err := gomodCmd.Output(); err == nil {
		gomod := strings.TrimSpace(string(b))
		if gomod != "" && gomod != "/dev/null" {
			modRoot := filepath.Dir(gomod)
			pkgPath := filepath.Join(modRoot, "cmd", "progressdb")
			if fi, err := os.Stat(pkgPath); err == nil && fi.IsDir() {
				build := exec.Command("go", "build", "-o", outPath, "./cmd/progressdb")
				build.Env = os.Environ()
				build.Dir = modRoot
				if bout, err := build.CombinedOutput(); err != nil {
					return fmt.Errorf("build from module root failed: %v\n%s", err, string(bout))
				}
				return nil
			}
		}
	}

	// Fallback: paths relative to cwd.
	cwd, _ := os.Getwd()
	candidates := []string{
		filepath.Join(cwd, ".."),       // when run from server/tests
		filepath.Join(cwd, "..", ".."), // from deeper paths
		cwd,
	}
	var lastErr error
	for _, dir := range candidates {
		pkgPath := filepath.Join(dir, "cmd", "progressdb")
		if fi, err := os.Stat(pkgPath); err == nil && fi.IsDir() {
			build := exec.Command("go", "build", "-o", outPath, "./cmd/progressdb")
			build.Env = os.Environ()
			build.Dir = dir
			if bout, err := build.CombinedOutput(); err != nil {
				lastErr = fmt.Errorf("build from %s failed: %v\n%s", dir, err, string(bout))
				continue
			}
			return nil
		}
	}
	if lastErr != nil {
		return lastErr
	}
	// Final build attempt, no Dir set.
	build := exec.Command("go", "build", "-o", outPath, "./cmd/progressdb")
	build.Env = os.Environ()
	if bout, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("final build attempt failed: %v\n%s", err, string(bout))
	}
	return nil
}

// Builds test binary at outPath, fails test on error.
// Convenience for tests outside utils package.
func BuildProgressdb(t *testing.T, outPath string) {
	t.Helper()
	if err := buildProgressdbBin(outPath); err != nil {
		t.Fatalf("failed to build progressdb binary: %v", err)
	}
}

// Builds (if needed) and starts the server. Waits for ready or fails on errors.
func StartServerProcess(t *testing.T, opts ServerOpts) *ServerProcess {
	t.Helper()

	workdir := NewArtifactsDir(t, "server-process")
	artifactRoot := TestArtifactsRoot(t)

	// Allocate port if requested.
	port := opts.Port
	if port == 0 {
		p, err := pickFreePort()
		if err != nil {
			t.Fatalf("pickFreePort: %v", err)
		}
		port = p
	}

	// Prepare config file.
	cfgPath := filepath.Join(workdir, "config.yaml")
	dbPath := filepath.Join(workdir, "db")
	if opts.ConfigYAML == "" {
		mk := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		cfg := fmt.Sprintf("server:\n  address: 127.0.0.1\n  port: %d\n  db_path: %s\nsecurity:\n  kms:\n    master_key_hex: %s\n  api_keys:\n    backend: []\n    frontend: []\n    admin: []\nlogging:\n  level: info\n", port, dbPath, mk)
		if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	} else {
		// Allow {{PORT}}, {{WORKDIR}} placeholders in ConfigYAML.
		cfgContent := opts.ConfigYAML
		if strings.Contains(cfgContent, "{{PORT}}") {
			cfgContent = strings.ReplaceAll(cfgContent, "{{PORT}}", strconv.Itoa(port))
		}
		if strings.Contains(cfgContent, "{{WORKDIR}}") {
			cfgContent = strings.ReplaceAll(cfgContent, "{{WORKDIR}}", workdir)
		}
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}

	// Build binary if needed. Prefer provided path, or TEST_BINARY/SERVER_BIN env vars.
	binPath := opts.BinaryPath
	if binPath == "" {
		// Check env overrides first.
		if p := os.Getenv("PROGRESSDB_TEST_BINARY"); p != "" {
			if _, err := os.Stat(p); err == nil {
				// Copy binary to local path.
				dst := filepath.Join(workdir, "progressdb-bin")
				if err := copyFile(p, dst); err == nil {
					binPath = dst
				}
			}
		}
		if binPath == "" {
			if p := os.Getenv("PROGRESSDB_SERVER_BIN"); p != "" {
				if _, err := os.Stat(p); err == nil {
					dst := filepath.Join(workdir, "progressdb-bin")
					if err := copyFile(p, dst); err == nil {
						binPath = dst
					}
				}
			}
		}
		if binPath == "" {
			binPath = filepath.Join(workdir, "progressdb-bin")
			// Build from sensible root (try go env GOMOD etc).
			if err := buildProgressdbBin(binPath); err != nil {
				t.Fatalf("failed to build server binary: %v", err)
			}
		}
	}

	// Substitute placeholders in env values ({{WORKDIR}}, {{PORT}})
	for k, v := range opts.Env {
		if strings.Contains(v, "{{WORKDIR}}") {
			opts.Env[k] = strings.ReplaceAll(v, "{{WORKDIR}}", workdir)
		}
		if strings.Contains(v, "{{PORT}}") {
			opts.Env[k] = strings.ReplaceAll(opts.Env[k], "{{PORT}}", strconv.Itoa(port))
		}
	}

	// Create stdout/stderr files.
	stdoutPath := filepath.Join(workdir, "stdout.log")
	stderrPath := filepath.Join(workdir, "stderr.log")
	stdoutF, err := os.Create(stdoutPath)
	if err != nil {
		t.Fatalf("create stdout file: %v", err)
	}
	stderrF, err := os.Create(stderrPath)
	if err != nil {
		t.Fatalf("create stderr file: %v", err)
	}

	// Start process.
	cmd := exec.Command(binPath, "--config", cfgPath)
	cmd.Stdout = io.MultiWriter(stdoutF)
	cmd.Stderr = io.MultiWriter(stderrF)
	if opts.Env == nil {
		opts.Env = map[string]string{}
	}
	if _, ok := opts.Env["PROGRESSDB_ARTIFACT_ROOT"]; !ok {
		opts.Env["PROGRESSDB_ARTIFACT_ROOT"] = artifactRoot
	}
	if _, ok := opts.Env["TEST_ARTIFACTS_ROOT"]; !ok {
		opts.Env["TEST_ARTIFACTS_ROOT"] = artifactRoot
	}
	cmd.Env = append(os.Environ(), envMapToSlice(opts.Env)...)
	cmd.Dir = workdir
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server process: %v", err)
	}

	sp := &ServerProcess{
		Addr:       fmt.Sprintf("http://127.0.0.1:%d", port),
		Cmd:        cmd,
		StdoutPath: stdoutPath,
		StderrPath: stderrPath,
		ConfigPath: cfgPath,
		WorkDir:    workdir,
		exitCh:     make(chan error, 1),
	}

	// Monitor process; record exit status to stderr for easier diagnostics.
	go func(c *exec.Cmd, sp *ServerProcess, outF, errF *os.File) {
		err := c.Wait()
		// Write exit info to stderr log.
		if f, ferr := os.OpenFile(sp.StderrPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600); ferr == nil {
			_, _ = fmt.Fprintf(f, "\n[%s] PROCESS EXIT: %v\n", time.Now().Format(time.RFC3339Nano), err)
			_ = f.Close()
		}
		// Close files after process exits.
		if outF != nil {
			_ = outF.Close()
		}
		if errF != nil {
			_ = errF.Close()
		}
		// Deliver exit to channel for Stop.
		select {
		case sp.exitCh <- err:
		default:
		}
	}(cmd, sp, stdoutF, stderrF)

	// Wait for ready (up to 1 minute).
	if err := waitForReady(sp.Addr, 1*time.Minute); err != nil {
		// Capture logs.
		stdout, _ := os.ReadFile(sp.StdoutPath)
		stderr, _ := os.ReadFile(sp.StderrPath)
		t.Fatalf("server failed to become ready: %v\nstdout:\n%s\nstderr:\n%s", err, string(stdout), string(stderr))
	}

	// Smoke check /healthz for handler responsiveness.
	healthOK := false
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, sp.Addr+"/healthz", nil)
		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err == nil && resp != nil {
			if resp.StatusCode == 200 {
				healthOK = true
			}
			_ = resp.Body.Close()
		}
		if healthOK {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !healthOK {
		stdout, _ := os.ReadFile(sp.StdoutPath)
		stderr, _ := os.ReadFile(sp.StderrPath)
		t.Fatalf("server readiness probe passed but /healthz did not respond OK\nstdout:\n%s\nstderr:\n%s", string(stdout), string(stderr))
	}

	// On test failure, print server logs to aid debugging.
	t.Cleanup(func() {
		if t.Failed() {
			if out, err := os.ReadFile(sp.StdoutPath); err == nil {
				t.Logf("---- server stdout (%s) ----\n%s", sp.StdoutPath, string(out))
			}
			if errb, err := os.ReadFile(sp.StderrPath); err == nil {
				t.Logf("---- server stderr (%s) ----\n%s", sp.StderrPath, string(errb))
			}
		}
	})

	t.Logf("started server at %s (workdir=%s)", sp.Addr, sp.WorkDir)
	return sp
}

// Stops process, returns exit error. Tries SIGINT, falls back to SIGKILL.
func (s *ServerProcess) Stop(t *testing.T) error {
	t.Helper()
	if s == nil || s.Cmd == nil || s.Cmd.Process == nil {
		return nil
	}
	// Send SIGINT.
	_ = s.Cmd.Process.Signal(syscall.SIGINT)
	// Wait for monitored exit, fallback to kill on timeout.
	select {
	case err := <-s.exitCh:
		if s != nil && s.StderrPath != "" {
			if f, ferr := os.OpenFile(s.StderrPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600); ferr == nil {
				_, _ = fmt.Fprintf(f, "\n[%s] PROCESS WAIT RETURNED: %v\n", time.Now().Format(time.RFC3339Nano), err)
				_ = f.Close()
			}
		}
		return err
	case <-time.After(5 * time.Second):
		// Force kill.
		_ = s.Cmd.Process.Kill()
		// Record forced kill.
		if s != nil && s.StderrPath != "" {
			if f, ferr := os.OpenFile(s.StderrPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600); ferr == nil {
				_, _ = fmt.Fprintf(f, "\n[%s] PROCESS KILLED AFTER TIMEOUT\n", time.Now().Format(time.RFC3339Nano))
				_ = f.Close()
			}
		}
		return fmt.Errorf("process killed after timeout")
	}
}

func pickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	a := l.Addr().(*net.TCPAddr)
	return a.Port, nil
}

func waitForReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := addr + "/readyz"
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err == nil {
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for ready: %s", addr)
}

func envMapToSlice(m map[string]string) []string {
	out := []string{}
	if m == nil {
		return out
	}
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
