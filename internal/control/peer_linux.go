// SPDX-License-Identifier: Apache-2.0

package control

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

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
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) // #nosec G115 -- a file descriptor fits in an int.
		if err != nil {
			credErr = err
			return
		}
		uid, pid = int(cred.Uid), int(cred.Pid)
	})
	if err != nil {
		return -1, 0, err
	}
	return uid, pid, credErr
}

// parentPID returns the parent process ID of pid, read from /proc.
func parentPID(pid int) (int, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	// The name in field 2 sits in parentheses and may hold spaces, so read
	// the fields after the last ")": state, then the parent ID.
	s := string(data)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0, fmt.Errorf("malformed /proc/%d/stat", pid)
	}
	fields := strings.Fields(s[i+1:])
	if len(fields) < 2 {
		return 0, fmt.Errorf("malformed /proc/%d/stat", pid)
	}
	return strconv.Atoi(fields[1])
}
