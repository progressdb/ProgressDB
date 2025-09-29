//go:build integration
// +build integration
package tests

import (
    "encoding/json"
    "testing"
    "time"

    "progressdb/internal/retention"
    "progressdb/pkg/models"
    "progressdb/pkg/store"
    "progressdb/pkg/utils"
)

func TestFileLeaseLifecycle(t *testing.T) {
    dir := t.TempDir()
    lock := retention.NewFileLease(dir)
    owner := utils.GenID()
    acq, err := lock.Acquire(owner, 2*time.Second)
    if err != nil {
        t.Fatalf("Acquire error: %v", err)
    }
    if !acq {
        t.Fatalf("expected to acquire lease")
    }
    // renew should succeed
    if err := lock.Renew(owner, 2*time.Second); err != nil {
        t.Fatalf("Renew error: %v", err)
    }
    // release should succeed
    if err := lock.Release(owner); err != nil {
        t.Fatalf("Release error: %v", err)
    }
}

func TestPurgeThreadIntegration(t *testing.T) {
    // set up pebble DB path
    dbdir := t.TempDir()
    if err := store.Open(dbdir); err != nil {
        t.Fatalf("store.Open: %v", err)
    }
    defer store.Close()

    // create thread metadata and save
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

    // ensure thread exists
    if s, err := store.GetThread(th.ID); err != nil || s == "" {
        t.Fatalf("GetThread failed before purge: %v s=%q", err, s)
    }

    // purge
    if err := store.PurgeThreadPermanently(th.ID); err != nil {
        t.Fatalf("PurgeThreadPermanently: %v", err)
    }

    // verify gone
    if s, err := store.GetThread(th.ID); err == nil && s != "" {
        t.Fatalf("expected thread to be removed; still present: %q", s)
    }
}

