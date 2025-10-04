package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// Paths holds canonical locations for runtime artifacts under a DB path.
type Paths struct {
	DB        string
	Store     string
	State     string
	Audit     string
	Retention string
	KMS       string
	Tmp       string
}

// PathsFor returns the canonical Paths for the provided DB path.
func PathsFor(dbPath string) Paths {
	statePath := filepath.Join(dbPath, "state")
	return Paths{
		DB:        dbPath,
		Store:     filepath.Join(dbPath, "store"),
		State:     statePath,
		Audit:     filepath.Join(statePath, "audit"),
		Retention: filepath.Join(statePath, "retention"),
		KMS:       filepath.Join(statePath, "kms"),
		Tmp:       filepath.Join(statePath, "tmp"),
	}
}

// Convenience helpers
func StorePath(dbPath string) string     { return PathsFor(dbPath).Store }
func StatePath(dbPath string) string     { return PathsFor(dbPath).State }
func AuditPath(dbPath string) string     { return PathsFor(dbPath).Audit }
func RetentionPath(dbPath string) string { return PathsFor(dbPath).Retention }
func KMSPath(dbPath string) string       { return PathsFor(dbPath).KMS }
func TmpPath(dbPath string) string       { return PathsFor(dbPath).Tmp }

// package-level cached paths after Init
var (
	// Paths is the canonical layout for the running process. Call Init once
	// at startup to populate it.
	PathsVar Paths
	initOnce sync.Once
)

// Init initializes the package-level Paths for the running process. Safe to
// call multiple times; initialization happens only once.
func Init(dbPath string) {
	initOnce.Do(func() {
		PathsVar = PathsFor(dbPath)
	})
}
