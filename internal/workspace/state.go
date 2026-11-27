// SPDX-License-Identifier: Apache-2.0

package workspace

import "fmt"

// State is where a workspace is in its life.
type State string

// The workspace states. A workspace starts in Creating.
const (
	// Creating: the branch and worktree are being made.
	Creating State = "creating"
	// Ready: the worktree exists and nothing runs in it.
	Ready State = "ready"
	// Archived: the worktree is gone and the branch is kept.
	Archived State = "archived"
	// Failed: a step failed. The error text says which.
	Failed State = "failed"
)

// transitions lists every allowed move. Anything not listed is refused.
var transitions = map[State][]State{
	Creating: {Ready, Failed},
	Ready:    {Archived, Failed},
	Failed:   {Archived},
	Archived: {},
}

// CanMove reports whether a workspace may go from one state to another.
func CanMove(from, to State) bool {
	for _, s := range transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

func checkMove(from, to State) error {
	if !CanMove(from, to) {
		return fmt.Errorf("a %s workspace cannot become %s", from, to)
	}
	return nil
}
