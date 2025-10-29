package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ensure canonical runtime folder layout exists under db path, not symlink, restrictive perms, writable
func EnsureStateDirs(dbPath string) error {
	storePath := filepath.Join(dbPath, "store")
	walPath := filepath.Join(dbPath, "wal")
	statePath := filepath.Join(dbPath, "state")
	auditPath := filepath.Join(statePath, "audit")
	retentionPath := filepath.Join(statePath, "retention")
	kmsPath := filepath.Join(statePath, "kms")
	tmpPath := filepath.Join(statePath, "tmp")
	telPath := filepath.Join(statePath, "telemetry")
	indexPath := filepath.Join(dbPath, "index")

	paths := []string{storePath, walPath, auditPath, retentionPath, kmsPath, tmpPath, telPath, indexPath}

	for _, p := range paths {
		// ensure parent exists
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return fmt.Errorf("cannot create parent for %s: %w", p, err)
		}

		// must be directory and not symlink if exists
		if fi, err := os.Lstat(p); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("path is a symlink: %s", p)
			}
			if !fi.IsDir() {
				return fmt.Errorf("path exists and is not a directory: %s", p)
			}
		}

		// create if missing
		if err := os.MkdirAll(p, 0o700); err != nil {
			return fmt.Errorf("cannot create path %s: %w", p, err)
		}

		// check not symlink after creation
		if fi2, err := os.Lstat(p); err == nil {
			if fi2.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("path is a symlink after creation: %s", p)
			}
		}

		// check writable by creating and deleting temp file
		tmp, err := os.CreateTemp(p, ".validate-*")
		if err != nil {
			return fmt.Errorf("path not writable: %s: %w", p, err)
		}
		tmp.Close()
		_ = os.Remove(tmp.Name())
	}

	return nil
}

// paths and helpers are defined in types.go
var (
	PathsVar Paths
	initOnce sync.Once
)

// cached paths after init
var initErr error

// safe to call multiple times; initialization happens once. ensures filesystem layout exists and returns error if any
func Init(dbPath string) error {
	initOnce.Do(func() {
		path := strings.TrimSpace(dbPath)
		if path == "" {
			path = "./database"
		}
		path = filepath.Clean(path)
		PathsVar = PathsFor(path)
		initErr = EnsureStateDirs(path)
	})
	return initErr
}
