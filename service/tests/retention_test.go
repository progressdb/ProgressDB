//go:build integration
// +build integration

package tests

// Objectives (from docs/tests.md):
// 1. Verify file-lease Acquire/Renew/Release semantics.
// 2. Verify purge behavior: soft-delete + purge window leads to permanent deletion.
// 3. Verify dry-run/audit modes write audit records without deleting data.
// 4. Tests should trigger retention deterministically (test trigger endpoint) rather than relying on timers.

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"progressdb/internal/retention"
	"progressdb/pkg/utils"
	testutils "progressdb/tests/utils"
)

func TestRetention_Suite(t *testing.T) {
	_ = testutils.TestArtifactsRoot(t)
	// Subtest: Test file lease acquire, renew, and release lifecycle semantics.
	t.Run("FileLeaseLifecycle", func(t *testing.T) {
		dir := testutils.NewArtifactsDir(t, "retention-lease")
		lock := retention.NewFileLease(dir)
		owner := storedb.GenMessageID()
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
		// Use public API only: create a thread, mark it deleted with an old
		// DeletedTS via the update endpoint, then trigger retention run and
		// assert the thread is removed.
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
logging:
  level: info
retention:
  enabled: true
  period: 24h
  dry_run: false`, utils.SigningSecret, utils.BackendAPIKey, utils.AdminAPIKey)
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// create thread
		user := "purge_user"
		tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "to-purge", 5*time.Second)

		// mark deleted with an old DeletedTS via update API
		old := time.Now().Add(-48 * time.Hour).UnixNano()
		up := map[string]interface{}{"deleted": true, "deleted_ts": old}
		status := utils.DoSignedJSON(t, sp.Addr, "PUT", "/v1/threads/"+tid, up, user, utils.FrontendAPIKey, nil)
		if status != 202 {
			t.Fatalf("unexpected update status: %d", status)
		}

		// wait until thread metadata reflects deletion
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			st, out := utils.GetThreadAPI(t, sp.Addr, user, tid)
			if st == 200 {
				if d, ok := out["deleted"].(bool); ok && d {
					break
				}
			}
			time.Sleep(200 * time.Millisecond)
		}

		// trigger retention run via admin test endpoint
		areq, _ := http.NewRequest("POST", sp.Addr+"/admin/test-retention-run", nil)
		areq.Header.Set("Authorization", "Bearer "+utils.AdminAPIKey)
		ares, err := http.DefaultClient.Do(areq)
		if err != nil {
			t.Fatalf("trigger retention failed: %v", err)
		}
		if ares.StatusCode != 200 {
			t.Fatalf("expected retention run to return 200; got %d", ares.StatusCode)
		}

		// thread should be removed; GET thread should return non-200
		st, _ := utils.GetThreadAPI(t, sp.Addr, user, tid)
		if st == 200 {
			t.Fatalf("expected thread to be purged; still visible")
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
		sp := utils.StartServerProcess(t, utils.ServerOpts{ConfigYAML: cfg})
		defer func() { _ = sp.Stop(t) }()

		// create a soft-deleted thread via API and set DeletedTS to the past
		user := "ret_user"
		tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "ret-thread", 5*time.Second)
		old := time.Now().Add(-48 * time.Hour).UnixNano()
		up := map[string]interface{}{"deleted": true, "deleted_ts": old}
		status := utils.DoSignedJSON(t, sp.Addr, "PUT", "/v1/threads/"+tid, up, user, utils.FrontendAPIKey, nil)
		if status != 202 {
			t.Fatalf("unexpected update status: %d", status)
		}

		// trigger retention via admin test endpoint (registered path)
		areq, _ := http.NewRequest("POST", sp.Addr+"/admin/test-retention-run", nil)
		areq.Header.Set("Authorization", "Bearer "+utils.AdminAPIKey)
		// admin API key is sufficient for /admin routes
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
		st, _ := utils.GetThreadAPI(t, sp.Addr, user, tid)
		if st != 200 {
			t.Fatalf("expected thread to remain after dry-run; got status=%d", st)
		}
	})
}
