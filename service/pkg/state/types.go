package state

import "path/filepath"

// Paths holds canonical locations for runtime artifacts under a DB path.
type Paths struct {
	DB        string
	Store     string
	Wal       string // dedicated WAL/root for durable queues
	State     string
	Audit     string
	Retention string
	KMS       string
	Tmp       string
	Tel       string
}

// PathsFor returns the canonical Paths for the provided DB path.
func PathsFor(dbPath string) Paths {
	statePath := filepath.Join(dbPath, "state")
	return Paths{
		DB:        dbPath,
		Store:     filepath.Join(dbPath, "store"),
		Wal:       filepath.Join(dbPath, "wal"),
		State:     statePath,
		Audit:     filepath.Join(statePath, "audit"),
		Retention: filepath.Join(statePath, "retention"),
		KMS:       filepath.Join(statePath, "kms"),
		Tmp:       filepath.Join(statePath, "tmp"),
		Tel:       filepath.Join(statePath, "tel"),
	}
}

// Convenience helpers
func StorePath(dbPath string) string     { return PathsFor(dbPath).Store }
func WalPath(dbPath string) string       { return PathsFor(dbPath).Wal }
func StatePath(dbPath string) string     { return PathsFor(dbPath).State }
func AuditPath(dbPath string) string     { return PathsFor(dbPath).Audit }
func RetentionPath(dbPath string) string { return PathsFor(dbPath).Retention }
func KMSPath(dbPath string) string       { return PathsFor(dbPath).KMS }
func TmpPath(dbPath string) string       { return PathsFor(dbPath).Tmp }
func TelPath(dbPath string) string       { return PathsFor(dbPath).Tel }
