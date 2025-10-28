//go:build linux || darwin || freebsd || netbsd || openbsd
// +build linux darwin freebsd netbsd openbsd

package encryption

import "golang.org/x/sys/unix"

// LockMemory locks the provided byte slice into RAM to avoid paging to disk.
func LockMemory(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	return unix.Mlock(b)
}

// UnlockMemory unlocks a previously locked memory region.
func UnlockMemory(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	return unix.Munlock(b)
}
