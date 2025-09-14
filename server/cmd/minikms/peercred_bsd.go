//go:build darwin || freebsd || netbsd || openbsd
// +build darwin freebsd netbsd openbsd

package main

import (
	"golang.org/x/sys/unix"
	"net"
)

// peerUIDForConn extracts the peer UID for a unix domain connection using
// getpeereid on BSD/macOS. Returns -1 on failure.
func peerUIDForConn(c net.Conn) int {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return -1
	}
	rc, err := uc.SyscallConn()
	if err != nil {
		return -1
	}
	var uid int = -1
	_ = rc.Control(func(fd uintptr) {
		euid, _, err := unix.Getpeereid(int(fd))
		if err != nil {
			return
		}
		uid = euid
	})
	return uid
}
