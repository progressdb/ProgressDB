package tests

import (
    "os"
    "path/filepath"
    "testing"

    "progressdb/pkg/logger"
)

func TestLogging_InitAndAttachAuditSink(t *testing.T) {
    // ensure init does not panic and sets global Log
    logger.Init()
    if logger.Log == nil {
        t.Fatalf("expected logger.Log to be non-nil after Init")
    }

    dir := t.TempDir()
    auditDir := filepath.Join(dir, "audit")
    if err := logger.AttachAuditFileSink(auditDir); err != nil {
        t.Fatalf("AttachAuditFileSink failed: %v", err)
    }
    // audit.log should exist
    fpath := filepath.Join(auditDir, "audit.log")
    if _, err := os.Stat(fpath); err != nil {
        t.Fatalf("expected audit log file to exist: %v", err)
    }
}

