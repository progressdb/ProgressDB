package tests

import (
	"bytes"
	"context"
	"encoding/json"
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

// Essential test constants
const (
	TestBackendKey  = "backend-key-123"
	TestFrontendKey = "frontend-key-123"
	TestAdminKey    = "admin-key-123"
	TestSigningKey  = "signing-key-123"
)

// API endpoints (will be updated when server starts)
var (
	EndpointBackendSign     string
	EndpointFrontendThreads string
	EndpointAdminHealth     string
	EndpointAdminKeys       string
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

// Test server process wrapper
type TestServer struct {
	*ServerProcess
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

// Builds (if needed) and starts server. Waits for ready or fails on errors.
func StartServerProcess(t *testing.T, opts ServerOpts) *ServerProcess {
	t.Helper()

	workdir := t.TempDir()

	// Allocate port if requested.
	port := opts.Port
	if port == 0 {
		p, err := pickFreePort()
		if err != nil {
			t.Fatalf("pickFreePort: %v", err)
		}
		port = p
	}

	// Generate unique keys for this test run to avoid rate limiter conflicts
	uniqueSuffix := strings.ReplaceAll(workdir, "/", "_")
	backendKey := "backend-key-" + uniqueSuffix[len(uniqueSuffix)-10:]
	frontendKey := "frontend-key-" + uniqueSuffix[len(uniqueSuffix)-10:]
	adminKey := "admin-key-" + uniqueSuffix[len(uniqueSuffix)-10:]
	signingKey := "signing-key-" + uniqueSuffix[len(uniqueSuffix)-10:]

	// Prepare config file.
	cfgPath := filepath.Join(workdir, "config.yaml")
	dbPath := filepath.Join(workdir, "db")
	t.Logf("opts.ConfigYAML = %q", opts.ConfigYAML)
	if opts.ConfigYAML == "" {
		t.Logf("Generating default config")
		mk := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		cfg := fmt.Sprintf("server:\n  address: 127.0.0.1\n  port: %d\n  db_path: %s\n  api_keys:\n    backend: [\"%s\"]\n    frontend: [\"%s\"]\n    admin: [\"%s\"]\n    signing: [\"%s\"]\n  rate_limit:\n    rps: 0\n    burst: 0\nencryption:\n  kms:\n    master_key_hex: %s\nlogging:\n  level: debug\n", port, dbPath, backendKey, frontendKey, adminKey, signingKey, mk)
		if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		// Debug: print config
		t.Logf("Test config:\n%s", cfg)
		// Also log the config file path for debugging
		t.Logf("Config file written to: %s", cfgPath)
	} else {
		// Allow {{PORT}}, {{WORKDIR}} placeholders in ConfigYAML.
		cfgContent := opts.ConfigYAML
		t.Logf("Original ConfigYAML: %s", cfgContent)
		if strings.Contains(cfgContent, "{{PORT}}") {
			cfgContent = strings.ReplaceAll(cfgContent, "{{PORT}}", strconv.Itoa(port))
		}
		if strings.Contains(cfgContent, "{{WORKDIR}}") {
			cfgContent = strings.ReplaceAll(cfgContent, "{{WORKDIR}}", workdir)
		}
		t.Logf("Processed ConfigYAML: %s", cfgContent)
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
	t.Logf("Starting server binary: %s", binPath)
	t.Logf("Server config: %s", cfgPath)
	t.Logf("Server working directory: %s", workdir)
	cmd := exec.Command(binPath, "--config", cfgPath)
	cmd.Stdout = io.MultiWriter(stdoutF)
	cmd.Stderr = io.MultiWriter(stderrF)
	if opts.Env == nil {
		opts.Env = map[string]string{}
	}
	cmd.Env = append(os.Environ(), envMapToSlice(opts.Env)...)
	cmd.Dir = workdir
	t.Logf("Server command: %s %v", binPath, cmd.Args)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server process: %v", err)
	}
	t.Logf("Server process started with PID: %d", cmd.Process.Pid)

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
			// Also write process state for debugging
			if c.ProcessState != nil {
				_, _ = fmt.Fprintf(f, "[%s] PROCESS EXITED: success=%v, code=%d\n", time.Now().Format(time.RFC3339Nano), c.ProcessState.Success(), c.ProcessState.ExitCode())
			}
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
	if t != nil {
		t.Helper()
	}
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

// StartTestServer starts a real ProgressDB server process for testing
func StartTestServer(t *testing.T) *TestServer {
	t.Helper()

	cfg := fmt.Sprintf(`server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
  api_keys:
    backend: ["%s"]
    frontend: ["%s"]
    admin: ["%s"]
    signing: ["%s"]
  rate_limit:
    rps: 1000
    burst: 1000
logging:
  level: debug
encryption:
  enabled: true
  fields: ["body.content"]
  kms:
    mode: embedded
    master_key_hex: d899a1b62c16e8a97f804f8c317c8c09eb758d2e231a4faf63dc882bc5f5ec3f
ingest:
  intake:
    queue_capacity: 1000
    wal:
      enabled: false
  compute:
    worker_count: 2
  apply:
    batch_timeout: 1s
telemetry:
  flush_interval: 2s
  buffer_size: 1MB
sensor:
  poll_interval: 1s
  disk_high_pct: 99
  mem_high_pct: 99
  cpu_high_pct: 99
  recovery_window: 10s`, TestBackendKey, TestFrontendKey, TestAdminKey, TestSigningKey)

	process := StartServerProcess(t, ServerOpts{ConfigYAML: cfg})

	// Update test endpoints with actual server address
	baseURL := strings.TrimSuffix(process.Addr, "/")
	EndpointBackendSign = baseURL + "/backend/v1/sign"
	EndpointFrontendThreads = baseURL + "/frontend/v1/threads"
	EndpointAdminHealth = baseURL + "/admin/health"
	EndpointAdminKeys = baseURL + "/admin/keys"

	return &TestServer{ServerProcess: process}
}

// Stop stops the test server process
func (ts *TestServer) Stop() {
	if ts.ServerProcess != nil {
		ts.ServerProcess.Stop(nil) // Pass nil for t since we're in cleanup
	}
}

// Simple HTTP helpers
func DoRequest(t *testing.T, method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequest(method, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return http.DefaultClient.Do(req)
}

func AuthHeaders(key string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + key,
	}
}

// SignedAuthHeaders generates headers with a valid user signature for frontend requests
func SignedAuthHeaders(frontendKey, userID string) (map[string]string, error) {
	// First, get a signature from the backend sign endpoint using backend key
	signBody := map[string]string{"userId": userID}
	jsonBody, _ := json.Marshal(signBody)

	// Use backend endpoint to get a signature
	baseURL := strings.TrimSuffix(EndpointBackendSign, "/backend/v1/sign")
	signURL := baseURL + "/backend/v1/sign"

	req, err := http.NewRequest("POST", signURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	// Use backend key for signing, but frontend key for the actual request
	req.Header.Set("Authorization", "Bearer "+TestBackendKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get signature: status %d", resp.StatusCode)
	}

	var signResp struct {
		UserID    string `json:"userId"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&signResp); err != nil {
		return nil, err
	}

	return map[string]string{
		"Authorization":    "Bearer " + frontendKey,
		"X-User-ID":        userID,
		"X-User-Signature": signResp.Signature,
	}, nil
}

func ThreadMessagesURL(threadID string) string {
	return EndpointFrontendThreads + "/" + threadID + "/messages"
}

// WithTestServer is a test helper that runs a test with a real server
func WithTestServer(t *testing.T, testFunc func()) {
	server := StartTestServer(t)
	defer server.Stop()
	testFunc()
}

// Utility functions
func SplitPath(p string) []string {
	out := make([]string, 0)
	cur := ""
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '/' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func MakeSlug(title, id string) string {
	t := strings.ToLower(title)
	var b strings.Builder
	lastDash := false
	for _, r := range t {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else {
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "t"
	}
	return fmt.Sprintf("%s-%s", s, id)
}

// Simple retry helper for async operations
func Retry(t *testing.T, maxAttempts int, delay time.Duration, fn func() bool) {
	t.Helper()
	for i := 0; i < maxAttempts; i++ {
		if fn() {
			return
		}
		time.Sleep(delay)
	}
	t.Fatalf("operation failed after %d attempts", maxAttempts)
}
