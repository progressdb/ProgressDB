//go:build linux
// +build linux

package main

import (
	"golang.org/x/sys/unix"
	"net"
)

// peerUIDForConn extracts the peer UID for a unix domain connection using
// SO_PEERCRED on Linux. Returns -1 on failure.
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
		ucred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			return
		}
		uid = int(ucred.Uid)
	})
	return uid
}
