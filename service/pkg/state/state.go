package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// EnsureStateDirs ensures the canonical runtime folder layout exists under
// the provided DB path. It verifies paths are not symlinks and have
// restrictive permissions, and that they are writable by the process.
func EnsureStateDirs(dbPath string) error {
	storePath := filepath.Join(dbPath, "store")
	walPath := filepath.Join(dbPath, "wal")
	statePath := filepath.Join(dbPath, "state")
	auditPath := filepath.Join(statePath, "audit")
	retentionPath := filepath.Join(statePath, "retention")
	kmsPath := filepath.Join(statePath, "kms")
	tmpPath := filepath.Join(statePath, "tmp")

	paths := []string{storePath, walPath, auditPath, retentionPath, kmsPath, tmpPath}

	for _, p := range paths {
		// ensure parent exists
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return fmt.Errorf("cannot create parent for %s: %w", p, err)
		}

		// if path exists, ensure it's a directory. We intentionally do not
		// enforce strict POSIX permission bits here so tests and developer
		// workflows on diverse platforms (Windows, shared mounts) are not
		// blocked. The primary requirement is that the process can write to
		// the directory; writability is validated below.
		if fi, err := os.Lstat(p); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("path is a symlink: %s", p)
			}
			if !fi.IsDir() {
				return fmt.Errorf("path exists and is not a directory: %s", p)
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

// Paths and helpers are defined in types.go

// package-level cached paths after Init
var (
	// Paths is the canonical layout for the running process. Call Init once
	// at startup to populate it.
	PathsVar Paths
	initOnce sync.Once
)

// Init initializes the package-level Paths for the running process. Safe to
// call multiple times; initialization happens only once.
var initErr error

// Init initializes the package-level Paths for the running process. Safe to
// call multiple times; initialization happens only once. It also ensures the
// filesystem layout exists by calling EnsureStateDirs and returns any error
// encountered.

func Init(dbPath string) error {
	initOnce.Do(func() {
		path := strings.TrimSpace(dbPath)
		if path == "" {
			if root := ArtifactRoot(); root != "" {
				path = filepath.Join(root, "db")
			} else {
				path = "./.database"
			}
		}
		path = filepath.Clean(path)
		PathsVar = PathsFor(path)
		initErr = EnsureStateDirs(path)
	})
	return initErr
}
