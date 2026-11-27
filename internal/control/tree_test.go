// SPDX-License-Identifier: Apache-2.0

package control

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestInTree(t *testing.T) {
	// An "agent": a shell in its own session, with a child that leaves the
	// parent chain by starting in the background.
	agent := exec.Command("sh", "-c", "sleep 30 & echo $!; wait")
	agent.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, err := agent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := agent.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-agent.Process.Pid, syscall.SIGKILL)
		_ = agent.Wait()
	})
	var child int
	buf := make([]byte, 32)
	_ = os.Stdout.Sync()
	deadline := time.Now().Add(5 * time.Second)
	for child == 0 && time.Now().Before(deadline) {
		n, _ := out.Read(buf)
		for _, c := range buf[:n] {
			if c >= '0' && c <= '9' {
				child = child*10 + int(c-'0')
			}
		}
	}
	if child == 0 {
		t.Fatal("no child pid")
	}

	roots := []int{agent.Process.Pid}
	if !InTree(agent.Process.Pid, roots) {
		t.Error("the agent itself is not in its tree")
	}
	if !InTree(child, roots) {
		t.Error("the agent's child is not in its tree")
	}
	if InTree(os.Getpid(), roots) {
		t.Error("the test process counts as the agent's")
	}
	if InTree(os.Getpid(), nil) {
		t.Error("with no agents, nothing is in a tree")
	}
	if !InTree(1<<30, roots) {
		t.Error("a process that cannot be looked up must count as in the tree")
	}
}

func TestParentPID(t *testing.T) {
	got, err := parentPID(os.Getpid())
	if err != nil || got != os.Getppid() {
		t.Errorf("parentPID = %d, %v, want %d", got, err, os.Getppid())
	}
}
