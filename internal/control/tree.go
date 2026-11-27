// SPDX-License-Identifier: Apache-2.0

package control

import (
	"slices"

	"golang.org/x/sys/unix"
)

// maxDepth caps the walk up the process tree.
const maxDepth = 64

// InTree reports whether process pid is one of roots or runs under one of
// them: by its session, which agents lead and their children keep, or by
// any parent up the tree. A process that cannot be looked up counts as in
// the tree, so a failed lookup never lets a caller through.
func InTree(pid int, roots []int) bool {
	if len(roots) == 0 {
		return false
	}
	if pid <= 0 {
		return true
	}
	if sid, err := unix.Getsid(pid); err != nil || slices.Contains(roots, sid) {
		return true
	}
	for range maxDepth {
		if slices.Contains(roots, pid) {
			return true
		}
		if pid <= 1 {
			return false
		}
		parent, err := parentPID(pid)
		if err != nil {
			return true
		}
		pid = parent
	}
	return true
}
