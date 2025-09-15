//go:build darwin || freebsd || netbsd || openbsd
// +build darwin freebsd netbsd openbsd

package conn

/*
#include <sys/types.h>
#include <sys/socket.h>
#include <unistd.h>
*/
import "C"

import (
	"net"
)

// PeerUIDForConn returns the peer UID for a unix domain connection on BSD/macOS
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
	fd := C.int(f.Fd())
	var uid C.uid_t
	var gid C.gid_t
	if rc := C.getpeereid(fd, &uid, &gid); rc != 0 {
		return -1
	}
	return int(uid)
}
