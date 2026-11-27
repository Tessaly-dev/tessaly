// SPDX-License-Identifier: Apache-2.0

package workspace

import "fmt"

// State is where a workspace is in its life.
type State string

// The workspace states. A workspace starts in Creating.
const (
	// Creating: the branch and worktree are being made.
	Creating State = "creating"
	// Ready: the worktree exists and no agent has run yet.
	Ready State = "ready"
	// Running: an agent runs in the worktree.
	Running State = "running"
	// Exited: the agent has exited. Its exit code is stored.
	Exited State = "exited"
	// Archived: the worktree is gone and the branch is kept.
	Archived State = "archived"
	// Failed: a step failed. The error text says which.
	Failed State = "failed"
)

// transitions lists every allowed move. Anything not listed is refused.
var transitions = map[State][]State{
	Creating: {Ready, Failed},
	Ready:    {Running, Archived, Failed},
	Running:  {Exited},
	Exited:   {Running, Archived},
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

// How an agent run ended, stored as the workspace's Ended field.
const (
	// EndExited: the agent exited on its own.
	EndExited = "exited"
	// EndStopped: the user stopped the agent.
	EndStopped = "stopped"
	// EndInterrupted: tess stopped, or crashed, while the agent ran.
	EndInterrupted = "interrupted"
)
