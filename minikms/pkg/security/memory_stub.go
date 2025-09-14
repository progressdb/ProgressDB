//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd
// +build !linux,!darwin,!freebsd,!netbsd,!openbsd

package security

// LockMemory is a no-op on unsupported platforms.
func LockMemory(b []byte) error { return nil }

// UnlockMemory is a no-op on unsupported platforms.
func UnlockMemory(b []byte) error { return nil }

