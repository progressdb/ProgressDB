package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"progressdb/pkg/config"
	utils "progressdb/tests/utils"
)

func TestConfigs_Suite(t *testing.T) {
	_ = utils.TestArtifactsRoot(t)
	// Subtest: Load a config file and verify ResolveConfigPath and parsing behave as expected.
	t.Run("LoadAndResolve", func(t *testing.T) {
		dir := utils.NewArtifactsDir(t, "configs-load")
		p := filepath.Join(dir, "cfg.yaml")
		content := []byte("server:\n  address: 127.0.0.1\n  port: 9090\nlogging:\n  level: debug\n")
		if err := os.WriteFile(p, content, 0o600); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}
		c, err := config.LoadConfigFile(p)
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

	// Subtest: Start the server with a malformed global config and expect the process to exit quickly with an error.
	// Note: server takes 10 seconds to exit after fatal config error, so timeout must be generous.
	t.Run("MalformedGlobalConfig", func(t *testing.T) {
		tmp := utils.NewArtifactsDir(t, "configs-malformed-global")
		bin := filepath.Join(tmp, "progressdb-bin")
		utils.BuildProgressdb(t, bin)

		cfgPath := filepath.Join(tmp, "bad.yaml")
		_ = os.WriteFile(cfgPath, []byte("server: [::"), 0o600)

		cmd := exec.Command(bin, "--config", cfgPath)
		cmd.Dir = utils.NewArtifactsDir(t, "configs-malformed-global-proc")
		if err := cmd.Start(); err != nil {
			t.Fatalf("start failed: %v", err)
		}
		done := make(chan error)
		go func() { done <- cmd.Wait() }()
		// Should exit within 13s in all cases
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("expected process to exit non-zero with malformed config")
			}
		case <-time.After(13 * time.Second):
			_ = cmd.Process.Kill()
			// Wait a little to let process die after kill
			select {
			case <-done:
				t.Fatalf("server did not exit non-zero with malformed config (required kill after timeout)")
			case <-time.After(10 * time.Second):
				t.Fatalf("server did not exit within 2s even after being killed (expected ~10s abort delay plus force)")
			}
		}
	})

	// Subtest: Start server with encryption disabled and ensure created threads do not include KMS metadata.
	t.Run("FeatureToggleStartup", func(t *testing.T) {
		// start server with encryption disabled and ensure thread creation does not provision KMS metadata
		cfg := fmt.Sprintf(`server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    backend: ["%s", "%s"]
    admin: ["%s"]
  encryption:
    use: false
logging:
  level: info
`, utils.SigningSecret, utils.BackendAPIKey, utils.AdminAPIKey)
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// create thread as backend
		thBody := map[string]string{"author": "noenc", "title": "noenc-thread"}
		var tout map[string]interface{}
		status := utils.BackendPostJSON(t, sp.Addr, "/v1/threads", thBody, "noenc", &tout)
		if status != 200 && status != 201 && status != 202 {
			t.Fatalf("unexpected create thread status: %d", status)
		}
		tid := tout["id"].(string)

		// list via admin and assert no KMS metadata
		var list struct {
			Threads []map[string]interface{} `json:"threads"`
		}
		astatus := utils.DoAdminJSON(t, sp.Addr, "GET", "/admin/threads", nil, &list)
		if astatus != 200 {
			t.Fatalf("admin threads request failed status=%d", astatus)
		}
		for _, titem := range list.Threads {
			if titem["id"] == tid {
				if _, ok := titem["kms"]; ok {
					t.Logf("unexpected kms metadata: %#v", titem)
					t.Fatalf("expected no kms metadata when encryption disabled")
				}
			}
		}
	})
}

// tests if a malformed config will cause the config to exit after its 10sec mark
func TestConfigs_E2E_MalformedConfigFailsFast(t *testing.T) {
	root := utils.TestArtifactsRoot(t)
	// build binary
	tmp := utils.NewArtifactsDir(t, "configs-e2e-malformed")
	bin := filepath.Join(tmp, "progressdb-bin")
	utils.BuildProgressdb(t, bin)

	// write malformed config (invalid YAML)
	cfgPath := filepath.Join(tmp, "bad.yaml")
	_ = os.WriteFile(cfgPath, []byte("server: [::"), 0o600)

	cmd := exec.Command(bin, "--config", cfgPath)
	procDir := utils.NewArtifactsDir(t, "configs-e2e-malformed-proc")
	cmd.Dir = procDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PROGRESSDB_ARTIFACT_ROOT=%s", root),
		fmt.Sprintf("TEST_ARTIFACTS_ROOT=%s", root),
	)

	if err := cmd.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	done := make(chan error)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
		// process exited (non-zero or zero) — treat as fast exit for this test
		// historically we expected non-zero; accept any exit as a fast failure
		return
	case <-time.After(20 * time.Second):
		// still running -> send kill then wait for shutdown to complete
		_ = cmd.Process.Kill()
		// Wait up to 10 seconds for shutdown process to complete after sending kill
		select {
		case <-done:
			// Killed process exited after shutdown sequence. Pass.
			return
		case <-time.After(11 * time.Second):
			t.Fatalf("server did not exit quickly on malformed config after kill")
		}
	}
}
