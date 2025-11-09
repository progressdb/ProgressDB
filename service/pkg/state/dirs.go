package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func EnsureStateDirs(dbPath string) error {
	storePath := filepath.Join(dbPath, "store")
	walPath := filepath.Join(dbPath, "wal")
	kmsPath := filepath.Join(dbPath, "kms")
	statePath := filepath.Join(dbPath, "state")
	auditPath := filepath.Join(statePath, "audit")
	retentionPath := filepath.Join(statePath, "retention")
	tmpPath := filepath.Join(statePath, "tmp")
	telPath := filepath.Join(statePath, "telemetry")
	logsPath := filepath.Join(statePath, "logs")
	indexPath := filepath.Join(dbPath, "index")
	backupsPath := filepath.Join(statePath, "backups")

	paths := []string{storePath, walPath, kmsPath, auditPath, retentionPath, tmpPath, telPath, logsPath, indexPath, backupsPath}

	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return fmt.Errorf("cannot create parent for %s: %w", p, err)
		}

		if fi, err := os.Lstat(p); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("path is a symlink: %s", p)
			}
			if !fi.IsDir() {
				return fmt.Errorf("path exists and is not a directory: %s", p)
			}
		}

		if err := os.MkdirAll(p, 0o700); err != nil {
			return fmt.Errorf("cannot create path %s: %w", p, err)
		}

		if fi2, err := os.Lstat(p); err == nil {
			if fi2.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("path is a symlink after creation: %s", p)
			}
		}

		tmp, err := os.CreateTemp(p, ".validate-*")
		if err != nil {
			return fmt.Errorf("path not writable: %s: %w", p, err)
		}
		tmp.Close()
		_ = os.Remove(tmp.Name())
	}

	return nil
}

var (
	PathsVar Paths
	initOnce sync.Once
)

var initErr error

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
