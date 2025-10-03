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

// ServerOpts contains options for starting the server process in tests.
type ServerOpts struct {
	// ConfigYAML if non-empty will be written to the temporary config file.
	// If empty a minimal config will be generated using Port and DBPath.
	ConfigYAML string
	// Port 0 picks a free port.
	Port int
	// Env additional environment variables to set for the server process.
	Env map[string]string
	// BinaryPath optional prebuilt binary to run. If empty the helper will build it.
	BinaryPath string
}

// ServerProcess represents a running test server process and related paths.
type ServerProcess struct {
	Addr       string // http://host:port
	Cmd        *exec.Cmd
	StdoutPath string
	StderrPath string
	ConfigPath string
	WorkDir    string
	exitCh     chan error
}

// StartServerProcess builds (if needed) and starts the server process using opts.
// It waits for readiness and fails the test on unrecoverable errors.
func StartServerProcess(t *testing.T, opts ServerOpts) *ServerProcess {
	t.Helper()

	workdir := t.TempDir()

	// allocate port if requested
	port := opts.Port
	if port == 0 {
		p, err := pickFreePort()
		if err != nil {
			t.Fatalf("pickFreePort: %v", err)
		}
		port = p
	}

	// prepare minimal config if none provided
	cfgPath := filepath.Join(workdir, "config.yaml")
	dbPath := filepath.Join(workdir, "db")
	if opts.ConfigYAML == "" {
		mk := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		cfg := fmt.Sprintf("server:\n  address: 127.0.0.1\n  port: %d\n  db_path: %s\nsecurity:\n  kms:\n    master_key_hex: %s\n  api_keys:\n    backend: []\n    frontend: []\n    admin: []\nlogging:\n  level: info\n", port, dbPath, mk)
		if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	} else {
		// allow using {{PORT}} placeholder in provided ConfigYAML
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

	// build binary if needed. Prefer prebuilt binary specified by opts.BinaryPath
	// or the PROGRESSDB_TEST_BINARY / PROGRESSDB_SERVER_BIN env var to speed up CI.
	binPath := opts.BinaryPath
	if binPath == "" {
		// check env overrides first
		if p := os.Getenv("PROGRESSDB_TEST_BINARY"); p != "" {
			if _, err := os.Stat(p); err == nil {
				binPath = p
			}
		}
		if binPath == "" {
			if p := os.Getenv("PROGRESSDB_SERVER_BIN"); p != "" {
				if _, err := os.Stat(p); err == nil {
					binPath = p
				}
			}
		}
		if binPath == "" {
			binPath = filepath.Join(workdir, "progressdb-bin")
			// try building from the server dir first (tests execute from server/tests),
			// then fall back to building from the repo root if that fails.
			build := exec.Command("go", "build", "-o", binPath, "./cmd/progressdb")
			build.Env = os.Environ()
			// run from the `server` directory (tests execute from server/tests)
			build.Dir = ".."
			bout, err := build.CombinedOutput()
			if err != nil {
				t.Fatalf("failed to build server binary: %v\noutput:\n%s", err, string(bout))
			}
		}
	}

	// substitute placeholders in env values ({{WORKDIR}}, {{PORT}})
	for k, v := range opts.Env {
		if strings.Contains(v, "{{WORKDIR}}") {
			opts.Env[k] = strings.ReplaceAll(v, "{{WORKDIR}}", workdir)
		}
		if strings.Contains(v, "{{PORT}}") {
			opts.Env[k] = strings.ReplaceAll(opts.Env[k], "{{PORT}}", strconv.Itoa(port))
		}
	}

	// prepare stdout/stderr files
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

	// start process
	cmd := exec.Command(binPath, "--config", cfgPath)
	cmd.Stdout = io.MultiWriter(stdoutF)
	cmd.Stderr = io.MultiWriter(stderrF)
	cmd.Env = append(os.Environ(), envMapToSlice(opts.Env)...)
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

	// monitor the child process and record its exit status to stderr log so
	// EOFs and unexpected terminations are easier to diagnose from test output.
	go func(c *exec.Cmd, sp *ServerProcess, outF, errF *os.File) {
		err := c.Wait()
		// try to append exit info to the stderr log file
		if f, ferr := os.OpenFile(sp.StderrPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600); ferr == nil {
			_, _ = fmt.Fprintf(f, "\n[%s] PROCESS EXIT: %v\n", time.Now().Format(time.RFC3339Nano), err)
			_ = f.Close()
		}
		// close parent-held file descriptors now that process exited
		if outF != nil {
			_ = outF.Close()
		}
		if errF != nil {
			_ = errF.Close()
		}
		// deliver exit to channel for Stop
		select {
		case sp.exitCh <- err:
		default:
		}
	}(cmd, sp, stdoutF, stderrF)

	// wait for readiness (give the server up to 1 minute to become healthy)
	if err := waitForReady(sp.Addr, 1*time.Minute); err != nil {
		// capture logs
		stdout, _ := os.ReadFile(sp.StdoutPath)
		stderr, _ := os.ReadFile(sp.StderrPath)
		t.Fatalf("server failed to become ready: %v\nstdout:\n%s\nstderr:\n%s", err, string(stdout), string(stderr))
	}

	// perform an additional smoke check against /healthz to ensure handlers are responsive
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

	// do not close files here; monitor goroutine will close them when the
	// child process exits to avoid races and "file already closed" errors.

	// register cleanup that prints server logs on test failure to aid debugging
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

// Stop stops the server process, returning its exit error if any. It will
// attempt graceful shutdown via SIGINT and fall back to SIGKILL.
func (s *ServerProcess) Stop(t *testing.T) error {
	t.Helper()
	if s == nil || s.Cmd == nil || s.Cmd.Process == nil {
		return nil
	}
	// send SIGINT
	_ = s.Cmd.Process.Signal(syscall.SIGINT)
	// Wait for the monitored exit result rather than calling Wait again to
	// avoid double-Wait races. If the process doesn't exit within the
	// timeout, attempt to kill it.
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
		// force kill
		_ = s.Cmd.Process.Kill()
		// record forced kill
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
