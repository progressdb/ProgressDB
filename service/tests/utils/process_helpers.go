package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"progressdb/pkg/state/logger"
	storedb "progressdb/pkg/store/db/store"
)

// StartTestServerProcess starts a test server with default config and test keys.
func StartTestServerProcess(t *testing.T) *ServerProcess {
	t.Helper()
	cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    backend: ["` + SigningSecret + `", "` + BackendAPIKey + `"]
    frontend: ["` + FrontendAPIKey + `"]
    admin: ["` + AdminAPIKey + `"]
logging:
  level: info
`
	return StartServerProcess(t, ServerOpts{ConfigYAML: cfg})
}

// PreseedDB creates a temp workdir, initializes the store, runs seedFn, returns workdir.
func PreseedDB(t *testing.T, prefix string, seedFn func(storePath string)) string {
	t.Helper()
	workdir := NewArtifactsDir(t, prefix)
	dbPath := filepath.Join(workdir, "db")
	storePath := filepath.Join(dbPath, "store")
	if err := os.MkdirAll(storePath, 0o700); err != nil {
		t.Fatalf("create store dir: %v", err)
	}

	logger.Init()

	// Open Pebble store with WAL disabled
	if err := storedb.Open(storePath, true, false); err != nil {
		t.Fatalf("storedb.Open failed: %v", err)
	}
	defer func() {
		if err := storedb.Close(); err != nil {
			t.Fatalf("storedb.Close failed: %v", err)
		}
	}()

	// Run test seed function
	if seedFn != nil {
		seedFn(storePath)
	}
	return workdir
}

// StartServerProcessWithWorkdir starts server using provided workdir.
// For use with PreseedDB.
func StartServerProcessWithWorkdir(t *testing.T, workdir string, opts ServerOpts) *ServerProcess {
	t.Helper()

	artifactRoot := TestArtifactsRoot(t)

	// pick free port if not given
	port := opts.Port
	if port == 0 {
		p, err := pickFreePort()
		if err != nil {
			t.Fatalf("pickFreePort: %v", err)
		}
		port = p
	}

	// Write config if not provided
	cfgPath := filepath.Join(workdir, "config.yaml")
	dbPath := filepath.Join(workdir, "db")
	if opts.ConfigYAML == "" {
		mk := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		cfg := fmt.Sprintf(
			"server:\n  address: 127.0.0.1\n  port: %d\n  db_path: %s\nsecurity:\n  kms:\n    master_key_hex: %s\n  api_keys:\n    backend: [\"%s\", \"%s\"]\n    frontend: [\"%s\"]\n    admin: [\"%s\"]\nlogging:\n  level: info\n",
			port, dbPath, mk, SigningSecret, BackendAPIKey, FrontendAPIKey, AdminAPIKey,
		)
		if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	} else {
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

	// Build or use provided server binary
	binPath := opts.BinaryPath
	if binPath == "" {
		if p := os.Getenv("PROGRESSDB_TEST_BINARY"); p != "" {
			if _, err := os.Stat(p); err == nil {
				dst := filepath.Join(workdir, "progressdb-bin")
				_ = copyFile(p, dst)
				binPath = dst
			}
		}
		if binPath == "" {
			if p := os.Getenv("PROGRESSDB_SERVER_BIN"); p != "" {
				if _, err := os.Stat(p); err == nil {
					dst := filepath.Join(workdir, "progressdb-bin")
					_ = copyFile(p, dst)
					binPath = dst
				}
			}
		}
		if binPath == "" {
			binPath = filepath.Join(workdir, "progressdb-bin")
			if err := buildProgressdbBin(binPath); err != nil {
				t.Fatalf("failed to build server binary: %v", err)
			}
		}
	}

	// Substitute placeholders in env
	for k, v := range opts.Env {
		if strings.Contains(v, "{{WORKDIR}}") {
			opts.Env[k] = strings.ReplaceAll(v, "{{WORKDIR}}", workdir)
		}
		if strings.Contains(v, "{{PORT}}") {
			opts.Env[k] = strings.ReplaceAll(opts.Env[k], "{{PORT}}", strconv.Itoa(port))
		}
	}

	// Set up stdout/stderr files
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

	// Launch process
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

	// Monitor process exit
	go func(c *exec.Cmd, sp *ServerProcess, outF, errF *os.File) {
		err := c.Wait()
		if f, ferr := os.OpenFile(sp.StderrPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600); ferr == nil {
			_, _ = fmt.Fprintf(f, "\n[%s] PROCESS EXIT: %v\n", time.Now().Format(time.RFC3339Nano), err)
			_ = f.Close()
		}
		if outF != nil {
			_ = outF.Close()
		}
		if errF != nil {
			_ = errF.Close()
		}
		select {
		case sp.exitCh <- err:
		default:
		}
	}(cmd, sp, stdoutF, stderrF)

	if err := waitForReady(sp.Addr, 1*time.Minute); err != nil {
		stdout, _ := os.ReadFile(sp.StdoutPath)
		stderr, _ := os.ReadFile(sp.StderrPath)
		t.Fatalf("server failed to become ready: %v\nstdout:\n%s\nstderr:\n%s", err, string(stdout), string(stderr))
	}

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

// WaitForReady polls the server's /readyz endpoint until it returns 200 or
// the timeout is reached. Fails the test on timeout.
func WaitForReady(t *testing.T, baseURL string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	url := baseURL + "/readyz"
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		cancel()
		if err == nil {
			if resp.StatusCode == 200 {
				_ = resp.Body.Close()
				return
			}
			_ = resp.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for ready: %s", baseURL)
}

// CreateThreadAPI creates a thread via the public API and returns the created thread id and
// the decoded response map. It fails the test on error.
func CreateThreadAPI(t *testing.T, baseURL, user, title string) (string, map[string]interface{}) {
	t.Helper()
	body := map[string]interface{}{"author": user, "title": title}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/threads", bytes.NewReader(b))
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-User-Signature", SignHMAC(SigningSecret, user))
	req.Header.Set("Authorization", "Bearer "+FrontendAPIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create thread request failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 201 && res.StatusCode != 202 {
		bodyBytes, _ := io.ReadAll(res.Body)
		t.Fatalf("unexpected create thread status: %d body=%s", res.StatusCode, string(bodyBytes))
	}
	var out map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("failed to decode create thread response: %v", err)
	}
	id, _ := out["id"].(string)
	return id, out
}

// CreateMessageAPI posts a message into a thread and returns the created message id.
func CreateMessageAPI(t *testing.T, baseURL, user, threadID string, body map[string]interface{}) string {
	t.Helper()
	if body == nil {
		body = map[string]interface{}{"author": user, "body": map[string]string{"text": "hello"}}
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/threads/"+threadID+"/messages", bytes.NewReader(b))
	req.Header.Set("X-User-ID", user)
	req.Header.Set("X-User-Signature", SignHMAC(SigningSecret, user))
	req.Header.Set("Authorization", "Bearer "+FrontendAPIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create message request failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 201 && res.StatusCode != 202 {
		bodyBytes, _ := io.ReadAll(res.Body)
		t.Fatalf("unexpected create message status: %d body=%s", res.StatusCode, string(bodyBytes))
	}
	var out map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("failed to decode create message response: %v", err)
	}
	id, _ := out["id"].(string)
	return id
}

// WaitForThreadVisible polls GET /v1/threads/{id} as the given user until the
// thread becomes visible (200) or timeout elapses.
func WaitForThreadVisible(t *testing.T, baseURL, id, user string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	url := baseURL + "/v1/threads/" + id
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("X-User-ID", user)
		req.Header.Set("X-User-Signature", SignHMAC(SigningSecret, user))
		req.Header.Set("Authorization", "Bearer "+FrontendAPIKey)
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		req = req.WithContext(ctx)
		res, err := http.DefaultClient.Do(req)
		cancel()
		if err == nil {
			if res.StatusCode == 200 {
				_ = res.Body.Close()
				return
			}
			_ = res.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for thread visible: %s", id)
}

// CreateThreadAndWait creates a thread via API and waits until it's visible.
func CreateThreadAndWait(t *testing.T, baseURL, user, title string, timeout time.Duration) (string, map[string]interface{}) {
	t.Helper()
	id, out := CreateThreadAPI(t, baseURL, user, title)
	WaitForThreadVisible(t, baseURL, id, user, timeout)
	return id, out
}

// GetThreadAPI fetches a thread by id as the given user and decodes the JSON response into out.
func GetThreadAPI(t *testing.T, baseURL, user, id string) (int, map[string]interface{}) {
	t.Helper()
	var out map[string]interface{}
	status := DoSignedJSON(t, baseURL, "GET", "/v1/threads/"+id, nil, user, FrontendAPIKey, &out)
	return status, out
}

// ListThreadsAPI lists threads for the given user (no query params) and decodes result.
func ListThreadsAPI(t *testing.T, baseURL, user string) (int, map[string]interface{}) {
	t.Helper()
	var out map[string]interface{}
	status := DoSignedJSON(t, baseURL, "GET", "/v1/threads", nil, user, FrontendAPIKey, &out)
	return status, out
}

// CreateMessageAndWait creates a message and waits until it's visible.
func CreateMessageAndWait(t *testing.T, baseURL, user, threadID string, body map[string]interface{}, timeout time.Duration) string {
	t.Helper()
	mid := CreateMessageAPI(t, baseURL, user, threadID, body)
	// wait until GET message returns 200
	deadline := time.Now().Add(timeout)
	path := "/v1/threads/" + threadID + "/messages/" + mid
	for time.Now().Before(deadline) {
		status := DoSignedJSON(t, baseURL, "GET", path, nil, user, FrontendAPIKey, nil)
		if status == 200 {
			return mid
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("message not visible after timeout: %s", mid)
	return ""
}

// DoSignedRequest performs an HTTP request to the test server using the
// provided API key and user identity. It returns the response and the body
// bytes (body is also available on resp.Body if callers prefer). The caller
// is responsible for closing resp.Body if not nil.
func DoSignedRequest(t *testing.T, baseURL, method, path string, body interface{}, user, apiKey string) (*http.Response, []byte) {
	t.Helper()
	var rb io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal body: %v", err)
		}
		rb = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+path, rb)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("X-User-ID", user)
		req.Header.Set("X-User-Signature", SignHMAC(SigningSecret, user))
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	// reset Body so callers can still read if they want
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return resp, bodyBytes
}

// DoSignedJSON performs a signed request and decodes a JSON response into
// out (pass pointer). It closes the response body. Fails the test on error.
func DoSignedJSON(t *testing.T, baseURL, method, path string, body interface{}, user, apiKey string, out interface{}) int {
	t.Helper()
	resp, data := DoSignedRequest(t, baseURL, method, path, body, user, apiKey)
	defer resp.Body.Close()
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("failed to unmarshal json response: %v (body=%s)", err, string(data))
		}
	}
	return resp.StatusCode
}

// DoAdminJSON is a convenience wrapper to call an admin-protected API using
// the test admin API key and decode JSON response into out. It fails the test
// on network or decode errors and returns the HTTP status code.
func DoAdminJSON(t *testing.T, baseURL, method, path string, body interface{}, out interface{}) int {
	t.Helper()
	return DoSignedJSON(t, baseURL, method, path, body, "", AdminAPIKey, out)
}

// GetAdminKey fetches a raw key value from the admin key endpoint and
// returns the HTTP status and body bytes. The caller is responsible for
// interpreting the bytes (admin endpoints return octet-stream for keys).
func GetAdminKey(t *testing.T, baseURL, key string) (int, []byte) {
	t.Helper()
	esc := url.PathEscape(key)
	path := "/admin/keys/" + esc
	resp, body := DoSignedRequest(t, baseURL, "GET", path, nil, "", AdminAPIKey)
	defer resp.Body.Close()
	return resp.StatusCode, body
}

// WaitForAdminKeyVisible polls the admin key endpoint until it returns 200
// or the timeout elapses. Fails the test on timeout.
func WaitForAdminKeyVisible(t *testing.T, baseURL, key string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, _ := GetAdminKey(t, baseURL, key)
		if status == 200 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for admin key visible: %s", key)
}
