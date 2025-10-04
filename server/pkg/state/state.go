package state

import (
    "fmt"
    "os"
    "path/filepath"
)

// EnsureStateDirs ensures the canonical runtime folder layout exists under
// the provided DB path. It verifies paths are not symlinks and have
// restrictive permissions, and that they are writable by the process.
func EnsureStateDirs(dbPath string) error {
    storePath := filepath.Join(dbPath, "store")
    statePath := filepath.Join(dbPath, "state")
    auditPath := filepath.Join(statePath, "audit")
    retentionPath := filepath.Join(statePath, "retention")
    kmsPath := filepath.Join(statePath, "kms")
    tmpPath := filepath.Join(statePath, "tmp")

    paths := []string{storePath, auditPath, retentionPath, kmsPath, tmpPath}

    for _, p := range paths {
        // ensure parent exists
        if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
            return fmt.Errorf("cannot create parent for %s: %w", p, err)
        }

        // if path exists, reject symlinks and non-directories
        if fi, err := os.Lstat(p); err == nil {
            if fi.Mode()&os.ModeSymlink != 0 {
                return fmt.Errorf("path is a symlink: %s", p)
            }
            if !fi.IsDir() {
                return fmt.Errorf("path exists and is not a directory: %s", p)
            }
            if fi.Mode().Perm()&0o022 != 0 {
                return fmt.Errorf("path has permissive mode (group/other write): %s", p)
            }
        }

        // create directory if missing
        if err := os.MkdirAll(p, 0o700); err != nil {
            return fmt.Errorf("cannot create path %s: %w", p, err)
        }

        // double-check no symlink after creation
        if fi2, err := os.Lstat(p); err == nil {
            if fi2.Mode()&os.ModeSymlink != 0 {
                return fmt.Errorf("path is a symlink after creation: %s", p)
            }
            if fi2.Mode().Perm()&0o022 != 0 {
                return fmt.Errorf("path has permissive mode after creation: %s", p)
            }
        }

        // writability check: create and remove a temp file
        tmp, err := os.CreateTemp(p, ".validate-*")
        if err != nil {
            return fmt.Errorf("path not writable: %s: %w", p, err)
        }
        tmp.Close()
        _ = os.Remove(tmp.Name())
    }

    return nil
}

