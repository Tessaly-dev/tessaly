// SPDX-License-Identifier: Apache-2.0

package control

import (
	"net"

	"golang.org/x/sys/unix"
)

// peer returns the user ID and process ID at the other end of c.
func peer(c *net.UnixConn) (uid, pid int, err error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return -1, 0, err
	}
	uid = -1
	var credErr error
	err = raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED) // #nosec G115 -- a file descriptor fits in an int.
		if err != nil {
			credErr = err
			return
		}
		uid = int(cred.Uid)
		pid, credErr = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID) // #nosec G115 -- a file descriptor fits in an int.
	})
	if err != nil {
		return -1, 0, err
	}
	return uid, pid, credErr
}

// parentPID returns the parent process ID of pid.
func parentPID(pid int) (int, error) {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, err
	}
	return int(info.Eproc.Ppid), nil
}
