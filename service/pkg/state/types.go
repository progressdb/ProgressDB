package state

import "path/filepath"

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
	Logs      string
	Index     string
	Crash     string // failed compute operations for recovery
}

func PathsFor(dbPath string) Paths {
	statePath := filepath.Join(dbPath, "state")
	return Paths{
		// base
		DB: dbPath,

		// mains
		Store: filepath.Join(dbPath, "store"),
		Index: filepath.Join(dbPath, "index"),
		Wal:   filepath.Join(dbPath, "wal"),
		KMS:   filepath.Join(dbPath, "kms"),

		// state
		State:     statePath,
		Audit:     filepath.Join(statePath, "audit"),
		Retention: filepath.Join(statePath, "retention"),
		Tmp:       filepath.Join(statePath, "tmp"),
		Tel:       filepath.Join(statePath, "telemetry"),
		Logs:      filepath.Join(statePath, "logs"),
		Crash:     filepath.Join(statePath, "crash"),
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
func IndexPath(dbPath string) string     { return PathsFor(dbPath).Index }
func CrashPath(dbPath string) string     { return PathsFor(dbPath).Crash }
