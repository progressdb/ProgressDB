//go:build integration
// +build integration

package tests

// Objectives (from docs/tests.md):
// 1. Verify file-lease Acquire/Renew/Release semantics.
// 2. Verify purge behavior: soft-delete + purge window leads to permanent deletion.
// 3. Verify dry-run/audit modes write audit records without deleting data.
// 4. Tests should trigger retention deterministically (test trigger endpoint) rather than relying on timers.

import (
	"encoding/json"
	"testing"
	"time"

	"progressdb/internal/retention"
	"progressdb/pkg/models"
	"progressdb/pkg/store"
	"progressdb/pkg/utils"
	testutils "progressdb/tests/utils"
)

func TestRetention_Suite(t *testing.T) {
	_ = testutils.TestArtifactsRoot(t)
	// Subtest: Test file lease acquire, renew, and release lifecycle semantics.
	t.Run("FileLeaseLifecycle", func(t *testing.T) {
		dir := testutils.NewArtifactsDir(t, "retention-lease")
		lock := retention.NewFileLease(dir)
		owner := utils.GenID()
		acq, err := lock.Acquire(owner, 2*time.Second)
		if err != nil {
			t.Fatalf("Acquire error: %v", err)
		}
		if !acq {
			t.Fatalf("expected to acquire lease")
		}
		if err := lock.Renew(owner, 2*time.Second); err != nil {
			t.Fatalf("Renew error: %v", err)
		}
		if err := lock.Release(owner); err != nil {
			t.Fatalf("Release error: %v", err)
		}
	})

	// Subtest: Verify purge integration removes soft-deleted thread permanently.
	t.Run("PurgeThreadIntegration", func(t *testing.T) {
		dbdir := testutils.NewArtifactsDir(t, "retention-db")
		storePath := filepath.Join(dbdir, "store")
		if err := os.MkdirAll(storePath, 0o700); err != nil {
			t.Fatalf("mkdir store path: %v", err)
		}
		if err := store.Open(storePath, true); err != nil {
			t.Fatalf("store.Open: %v", err)
		}
		defer store.Close()

		th := models.Thread{
			ID:        "thread-test-1",
			Title:     "t",
			Deleted:   true,
			DeletedTS: time.Now().Add(-48 * time.Hour).UnixNano(),
		}
		b, _ := json.Marshal(th)
		if err := store.SaveThread(th.ID, string(b)); err != nil {
			t.Fatalf("SaveThread: %v", err)
		}
		if s, err := store.GetThread(th.ID); err != nil || s == "" {
			t.Fatalf("GetThread failed before purge: %v s=%q", err, s)
		}
		if err := store.PurgeThreadPermanently(th.ID); err != nil {
			t.Fatalf("PurgeThreadPermanently: %v", err)
		}
		if s, err := store.GetThread(th.ID); err == nil && s != "" {
			t.Fatalf("expected thread to be removed; still present: %q", s)
		}
	})

	// Subtest: Run retention in dry-run mode and verify audit entries are written while data remains.
	t.Run("DryRunAudit", func(t *testing.T) {
		// start server with retention enabled in dry-run and testing env enabled
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
retention:
  enabled: true
  period: 1s
  dry_run: true
`
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg, Env: map[string]string{"PROGRESSDB_TESTING": "1"}})
		defer func() { _ = sp.Stop(t) }()

		// create a soft-deleted thread older than period
		th := models.Thread{
			ID:        "ret-test-1",
			Title:     "t",
			Deleted:   true,
			DeletedTS: time.Now().Add(-48 * time.Hour).UnixNano(),
		}
		b, _ := json.Marshal(th)
		// save via store directly - the store is safe to open separately for tests
		if err := store.SaveThread(th.ID, string(b)); err != nil {
			t.Fatalf("SaveThread: %v", err)
		}

		// trigger retention via admin test endpoint
		areq, _ := http.NewRequest("POST", sp.Addr+"/admin/test/retention-run", nil)
		areq.Header.Set("Authorization", "Bearer admin-secret")
		ares, err := http.DefaultClient.Do(areq)
		if err != nil {
			t.Fatalf("trigger retention failed: %v", err)
		}
		if ares.StatusCode != 200 {
			t.Fatalf("expected retention run to return 200; got %d", ares.StatusCode)
		}

		// audit.log should exist under db/state/audit/audit.log
		auditPath := filepath.Join(sp.WorkDir, "db", "state", "audit", "audit.log")
		deadline := time.Now().Add(5 * time.Second)
		found := false
		for time.Now().Before(deadline) {
			if data, err := os.ReadFile(auditPath); err == nil {
				if len(data) > 0 && bytes.Contains(data, []byte("retention_audit_item")) {
					found = true
					break
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !found {
			t.Fatalf("expected audit entry in %s", auditPath)
		}

		// in dry-run, thread should still exist
		if s, err := store.GetThread(th.ID); err != nil || s == "" {
			t.Fatalf("expected thread to remain after dry-run; got err=%v s=%q", err, s)
		}
	})
}
