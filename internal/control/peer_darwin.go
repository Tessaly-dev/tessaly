// SPDX-License-Identifier: Apache-2.0

package control

import (
	"net"

	"golang.org/x/sys/unix"
)

// peerUID returns the user ID of the process at the other end of c.
func peerUID(c *net.UnixConn) (int, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return -1, err
	}
	uid := -1
	var credErr error
	err = raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED) // #nosec G115 -- a file descriptor fits in an int.
		if err != nil {
			credErr = err
			return
		}
		uid = int(cred.Uid)
	})
	if err != nil {
		return -1, err
	}
	return uid, credErr
}
