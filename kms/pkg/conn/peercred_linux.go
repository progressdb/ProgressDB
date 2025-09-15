//go:build linux
// +build linux

package conn

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// PeerUIDForConn returns the peer UID for a unix domain connection on Linux
// or -1 when unavailable.
func PeerUIDForConn(c net.Conn) int {
	uconn, ok := c.(*net.UnixConn)
	if !ok {
		return -1
	}
	f, err := uconn.File()
	if err != nil {
		return -1
	}
	defer f.Close()
	fd := int(f.Fd())
	cred, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		_ = os.NewSyscallError("getsockopt", err)
		return -1
	}
	return int(cred.Uid)
}
