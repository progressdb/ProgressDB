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

	"progressdb/pkg/logger"
	utils "progressdb/tests/utils"
)

func TestLogging_Suite(t *testing.T) {
	t.Run("InitAndAttachAuditSink", func(t *testing.T) {
		logger.Init()
		if logger.Log == nil {
			t.Fatalf("expected logger.Log to be non-nil after Init")
		}

		dir := t.TempDir()
		auditDir := filepath.Join(dir, "audit")
		if err := logger.AttachAuditFileSink(auditDir); err != nil {
			t.Fatalf("AttachAuditFileSink failed: %v", err)
		}
		fpath := filepath.Join(auditDir, "audit.log")
		if _, err := os.Stat(fpath); err != nil {
			t.Fatalf("expected audit log file to exist: %v", err)
		}
	})

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
  level: info
`
		// write logs to file and set debug level
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg, Env: map[string]string{"PROGRESSDB_LOG_SINK": "file:{{WORKDIR}}/app.log", "PROGRESSDB_LOG_LEVEL": "debug"}})
		defer func() { _ = sp.Stop(t) }()

		// make a request that will trigger auth_check debug log
		_, _ = http.Get(sp.Addr + "/healthz")

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

	t.Run("AuditPathFailureModes", func(t *testing.T) {
		// create a tmp dir and a file at <db>/retention to provoke MkdirAll failure
		tmp := t.TempDir()
		db := filepath.Join(tmp, "db")
		_ = os.MkdirAll(db, 0o700)
		// create a file where the retention directory should be
		bad := filepath.Join(db, "retention")
		if err := os.WriteFile(bad, []byte("not a dir"), 0o600); err != nil {
			t.Fatalf("write bad retention file: %v", err)
		}

		// build binary
		bin := filepath.Join(tmp, "progressdb-bin")
		build := exec.Command("go", "build", "-o", bin, "./server/cmd/progressdb")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build failed: %v\n%s", err, string(out))
		}

		cfg := filepath.Join(tmp, "cfg.yaml")
		conf := []byte("server:\n  address: 127.0.0.1\n  port: 0\n  db_path: " + db + "\nlogging:\n  level: info\n")
		if err := os.WriteFile(cfg, conf, 0o600); err != nil {
			t.Fatalf("write cfg: %v", err)
		}

		// start process and capture stdout
		cmd := exec.Command(bin, "--config", cfg)
		outf := filepath.Join(tmp, "out.log")
		of, _ := os.Create(outf)
		cmd.Stdout = of
		cmd.Stderr = of
		if err := cmd.Start(); err != nil {
			t.Fatalf("start server failed: %v", err)
		}
		// wait briefly for attach_audit_sink_failed
		time.Sleep(500 * time.Millisecond)
		_ = cmd.Process.Kill()
		of.Close()
		data, _ := os.ReadFile(outf)
		if !bytes.Contains(data, []byte("attach_audit_sink_failed")) {
			t.Fatalf("expected attach_audit_sink_failed in output; got:\n%s", string(data))
		}
	})

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
				_, _ = http.Get(sp.Addr + "/healthz")
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
	areq, _ := http.NewRequest("POST", sp.Addr+"/admin/encryption/rotate-thread-dek", bytes.NewReader([]byte(`{"thread_id":"nonexistent"}`)))
	areq.Header.Set("Authorization", "Bearer admin-secret")
	ares, err := http.DefaultClient.Do(areq)
	if err != nil {
		t.Fatalf("admin rotate request failed: %v", err)
	}
	// allow 200 or error; we only care audit file created
	_ = ares

	// audit.log is under <DBPath>/retention/audit.log per main attaching behavior
	auditPath := filepath.Join(sp.WorkDir, "db", "retention", "audit.log")
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
