package tests

// Objectives (from docs/tests.md):
// 1. Spawn the server with malformed config - does it fail fast?
// 2. Spawn the server with malformed per-feature config - does it fail fast?
// 3. Spawn the server with features toggled on/off and verify correct startup.
// 4. Spawn the server with all features enabled and verify full surface.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"progressdb/pkg/config"
	utils "progressdb/tests/utils"
)

func TestConfigs_Suite(t *testing.T) {
	t.Run("LoadAndResolve", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "cfg.yaml")
		content := []byte("server:\n  address: 127.0.0.1\n  port: 9090\nlogging:\n  level: debug\n")
		if err := os.WriteFile(p, content, 0o600); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}
		c, err := config.Load(p)
		if err != nil {
			t.Fatalf("config.Load failed: %v", err)
		}
		if c.Server.Port != 9090 {
			t.Fatalf("expected port 9090 got %d", c.Server.Port)
		}

		os.Setenv("PROGRESSDB_SERVER_CONFIG", p)
		defer os.Unsetenv("PROGRESSDB_SERVER_CONFIG")
		got := config.ResolveConfigPath("/nope", false)
		if got != p {
			t.Fatalf("ResolveConfigPath expected %q got %q", p, got)
		}
	})

	t.Run("MalformedGlobalConfig", func(t *testing.T) {
		// build binary and start with malformed config
		tmp := t.TempDir()
		bin := filepath.Join(tmp, "progressdb-bin")
		// try building from the server dir first, then fall back to building from repo root
		build := exec.Command("go", "build", "-o", bin, "./cmd/progressdb")
		build.Env = os.Environ()
		// run build from the `server` directory (tests execute from server/tests)
		build.Dir = ".."
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build failed: %v\n%s", err, string(out))
		}

		cfgPath := filepath.Join(tmp, "bad.yaml")
		_ = os.WriteFile(cfgPath, []byte("server: [::"), 0o600)

		cmd := exec.Command(bin, "--config", cfgPath)
		if err := cmd.Start(); err != nil {
			t.Fatalf("start failed: %v", err)
		}
		done := make(chan error)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("expected process to exit non-zero with malformed config")
			}
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatalf("server did not exit quickly on malformed config")
		}
	})

	t.Run("FeatureToggleStartup", func(t *testing.T) {
		// start server with encryption disabled and ensure thread creation does not provision KMS metadata
		cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    admin: ["admin-secret"]
  encryption:
    use: false
logging:
  level: info
`

		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// create thread as admin
		thBody := map[string]string{"author": "noenc", "title": "noenc-thread"}
		tb, _ := json.Marshal(thBody)
		req, _ := http.NewRequest("POST", sp.Addr+"/v1/threads", bytes.NewReader(tb))
		req.Header.Set("Authorization", "Bearer admin-secret")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("create thread failed: %v", err)
		}
		if res.StatusCode != 200 && res.StatusCode != 201 {
			t.Fatalf("unexpected create thread status: %d", res.StatusCode)
		}
		var tout map[string]interface{}
		_ = json.NewDecoder(res.Body).Decode(&tout)
		tid := tout["id"].(string)

		// list via admin and assert no KMS metadata
		areq, _ := http.NewRequest("GET", sp.Addr+"/admin/threads", nil)
		areq.Header.Set("Authorization", "Bearer admin-secret")
		ares, err := http.DefaultClient.Do(areq)
		if err != nil {
			t.Fatalf("admin threads request failed: %v", err)
		}
		var list struct {
			Threads []map[string]interface{} `json:"threads"`
		}
		_ = json.NewDecoder(ares.Body).Decode(&list)
		for _, titem := range list.Threads {
			if titem["id"] == tid {
				if _, ok := titem["kms"]; ok {
					t.Fatalf("expected no kms metadata when encryption disabled")
				}
			}
		}
	})
}

func TestConfigs_E2E_MalformedConfigFailsFast(t *testing.T) {
	// build binary
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "progressdb-bin")
	// try building from the server dir first, then fall back to building from repo root
	build := exec.Command("go", "build", "-o", bin, "./cmd/progressdb")
	build.Env = os.Environ()
	// run build from the `server` directory (tests execute from server/tests)
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, string(out))
	}

	// write malformed config
	cfgPath := filepath.Join(tmp, "bad.yaml")
	_ = os.WriteFile(cfgPath, []byte("::not yaml::"), 0o600)

	cmd := exec.Command(bin, "--config", cfgPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	done := make(chan error)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected process to exit non-zero with malformed config")
		}
	case <-time.After(5 * time.Second):
		// still running -> fail
		_ = cmd.Process.Kill()
		t.Fatalf("server did not exit quickly on malformed config")
	}
}
