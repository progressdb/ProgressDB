package tests

// Objectives (from docs/tests.md):
// 1. Start server with default logging configuration — verify startup banner and basic Info logging to stdout.
// 2. Start server with an audit sink configured — verify an audit file is created and audit JSON lines are emitted.
// 3. Validate log level behavior (DEBUG/INFO/WARN/ERROR) via configuration.
// 4. Validate failure modes (e.g., audit path permission error) surface during startup or fall back gracefully.
// 5. Smoke test concurrent logging during load and verify no panics and logs are produced.

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"progressdb/pkg/state/logger"
	utils "progressdb/tests/utils"
)

func TestLogging_Suite(t *testing.T) {
	_ = utils.TestArtifactsRoot(t)
	// subtest: Initialize logger and attach audit file sink; verify audit file created.
	t.Run("InitAndAttachAuditSink", func(t *testing.T) {
		logger.Init()
		if logger.Log == nil {
			t.Fatalf("expected logger.Log to be non-nil after Init")
		}

		dir := utils.NewArtifactsDir(t, "logging-audit")
		auditDir := filepath.Join(dir, "audit")
		if err := logger.AttachAuditFileSink(auditDir); err != nil {
			t.Fatalf("AttachAuditFileSink failed: %v", err)
		}
		fpath := filepath.Join(auditDir, "audit.log")
		if _, err := os.Stat(fpath); err != nil {
			t.Fatalf("expected audit log file to exist: %v", err)
		}
	})

	// subtest: Start server with debug log level and verify debug entries appear in log.
	t.Run("LogLevelBehavior", func(t *testing.T) {
		cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    backend: []
    frontend: []
    admin: []
logging:
  # leave empty so PROGRESSDB_LOG_LEVEL env can control runtime level
  level: ""
`
		// write logs to file and set debug level
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg, Env: map[string]string{"PROGRESSDB_LOG_SINK": "file:{{WORKDIR}}/app.log", "PROGRESSDB_LOG_LEVEL": "debug"}})
		defer func() { _ = sp.Stop(t) }()

		// make a request that will trigger auth_check debug log
		if _, err := http.Get(sp.Addr + "/healthz"); err != nil {
			t.Fatalf("healthz request failed: %v", err)
		}

		// check log file for debug entry
		logPath := filepath.Join(sp.WorkDir, "app.log")
		deadline := time.Now().Add(5 * time.Second)
		found := false
		for time.Now().Before(deadline) {
			if data, err := os.ReadFile(logPath); err == nil {
				if bytes.Contains(data, []byte("auth_check")) || bytes.Contains(data, []byte("request_allowed")) {
					found = true
					break
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !found {
			t.Fatalf("expected debug auth_check entry in log %s", logPath)
		}
	})

	// subtest: Start server with an invalid audit/audit path and verify failure is logged (attach_audit_sink_failed).
	t.Run("AuditPathFailureModes", func(t *testing.T) {
		// create a tmp dir and a file at <db>/state/audit to provoke MkdirAll failure
		tmp := utils.NewArtifactsDir(t, "logging-audit-failure")
		db := filepath.Join(tmp, "db")
		_ = os.MkdirAll(db, 0o700)
		// ensure parent state dir exists, then create a file where the audit
		// directory should be so AttachAuditFileSink sees an existing file.
		stateDir := filepath.Join(db, "state")
		_ = os.MkdirAll(stateDir, 0o700)
		bad := filepath.Join(stateDir, "audit")
		if err := os.WriteFile(bad, []byte("not a dir"), 0o600); err != nil {
			t.Fatalf("write bad audit file: %v", err)
		}

		// build binary
		bin := filepath.Join(tmp, "progressdb-bin")
		// try building from the server dir first, then fall back to building from repo root
		// build binary using test helper to locate repo root reliably
		utils.BuildProgressdb(t, bin)

		// create minimal config
		cfg := filepath.Join(tmp, "cfg.yaml")
		conf := []byte("server:\n  address: 127.0.0.1\n  port: 0\n  db_path: " + db + "\nlogging:\n  level: info\n")
		if err := os.WriteFile(cfg, conf, 0o600); err != nil {
			t.Fatalf("write cfg: %v", err)
		}

		// run process (blocking) and capture stdout/stderr
		cmd := exec.Command(bin, "--config", cfg)
		cmd.Dir = utils.NewArtifactsDir(t, "logging-audit-failure-proc")
		outf := filepath.Join(tmp, "out.log")
		of, _ := os.Create(outf)
		cmd.Stdout = of
		cmd.Stderr = of
		err := cmd.Run()
		of.Close()
		data, _ := os.ReadFile(outf)
		if err == nil {
			t.Fatalf("expected server to exit non-zero when audit path is broken; output:\n%s", string(data))
		}
		if !bytes.Contains(data, []byte("state_dirs_setup_failed")) {
			t.Fatalf("expected state_dirs_setup_failed in output; got:\n%s", string(data))
		}
	})

	// (removed preflight subtest — filesystem checks are now run during Init at startup)

	// subtest: Fire many concurrent requests to exercise logger under concurrency; ensure no panics and logs produced.
	t.Run("ConcurrentLoggingSmoke", func(t *testing.T) {
		cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    admin: ["admin-secret"]
logging:
  level: info
`
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg, Env: map[string]string{"PROGRESSDB_LOG_SINK": "file:{{WORKDIR}}/log.txt", "PROGRESSDB_LOG_LEVEL": "info"}})
		defer func() { _ = sp.Stop(t) }()

		// fire many concurrent requests
		n := 20
		done := make(chan struct{}, n)
		for i := 0; i < n; i++ {
			go func() {
				if _, err := http.Get(sp.Addr + "/healthz"); err != nil {
					t.Logf("healthz request error in concurrent logger test: %v", err)
				}
				done <- struct{}{}
			}()
		}
		for i := 0; i < n; i++ {
			<-done
		}

		// ensure log file exists and non-empty
		logPath := filepath.Join(sp.WorkDir, "log.txt")
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read log file failed: %v", err)
		}
		if len(data) == 0 {
			t.Fatalf("expected non-empty log file")
		}
	})
}

func TestLogging_E2E_AuditFile(t *testing.T) {
	_ = utils.TestArtifactsRoot(t)
	// start server with admin key so we can call an admin endpoint that emits audit logs
	cfg := `server:
  address: 127.0.0.1
  port: {{PORT}}
  db_path: {{WORKDIR}}/db
security:
  kms:
    master_key_hex: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  api_keys:
    backend: []
    frontend: []
    admin: ["admin-secret"]
logging:
  level: info
`
	sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
	defer func() { _ = sp.Stop(t) }()

	// call an admin endpoint that will create server events (rotate DEK on a non-existing thread will still log)
	areq, _ := http.NewRequest("POST", sp.Addr+"/admin/encryption/rotate-thread-dek", bytes.NewReader([]byte(`{"thread_key":"nonexistent"}`)))
	areq.Header.Set("Authorization", "Bearer "+utils.AdminAPIKey)
	// admin API key is sufficient for /admin routes
	ares, err := http.DefaultClient.Do(areq)
	if err != nil {
		t.Fatalf("admin rotate request failed: %v", err)
	}
	// allow 200 or error; we only care audit file created
	_ = ares

	// audit.log is under <DBPath>/state/audit/audit.log per main attaching behavior
	auditPath := filepath.Join(sp.WorkDir, "db", "state", "audit", "audit.log")
	// wait for file to appear
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(auditPath); err == nil {
			// quick sanity check: file non-empty
			if fi, _ := os.Stat(auditPath); fi.Size() > 0 {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("expected audit.log to be created at %s", auditPath)
}
