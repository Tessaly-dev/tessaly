// SPDX-License-Identifier: Apache-2.0

package ptyx

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// wait fails the test if ch does not close within 10 s.
func wait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
}

type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestOutputAndExitCode(t *testing.T) {
	var transcript safeBuffer
	p, err := Start([]string{"sh", "-c", "echo hello from $NAME; exit 3"}, t.TempDir(), []string{"NAME=pty", "PATH=" + os.Getenv("PATH")}, &transcript)
	if err != nil {
		t.Fatal(err)
	}
	wait(t, p.Done())

	if got := string(p.Output()); !strings.Contains(got, "hello from pty") {
		t.Errorf("output = %q", got)
	}
	if !strings.Contains(transcript.String(), "hello from pty") {
		t.Errorf("transcript = %q", transcript.String())
	}
	if code := p.ExitCode(); code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestRunsInDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/marker", []byte("here"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := Start([]string{"cat", "marker"}, dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wait(t, p.Done())
	if got := string(p.Output()); !strings.Contains(got, "here") {
		t.Errorf("output = %q, want the marker file", got)
	}
}

func TestStartMissingProgram(t *testing.T) {
	if _, err := Start([]string{"/no/such/program"}, t.TempDir(), nil, nil); err == nil {
		t.Error("Start of a missing program succeeded")
	}
	if _, err := Start(nil, t.TempDir(), nil, nil); err == nil {
		t.Error("Start of an empty argv succeeded")
	}
}

func TestWriteAndSubscribe(t *testing.T) {
	p, err := Start([]string{"cat"}, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop(context.Background(), time.Second)

	_, updates, cancel := p.Subscribe()
	defer cancel()
	if _, err := p.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}

	var got bytes.Buffer
	deadline := time.After(10 * time.Second)
	for !strings.Contains(got.String(), "ping") {
		select {
		case b, ok := <-updates:
			if !ok {
				t.Fatal("updates closed early")
			}
			got.Write(b)
		case <-deadline:
			t.Fatalf("no echo, got %q", got.String())
		}
	}

	cancel()
	cancel() // Safe to call twice.
	if _, ok := <-updates; ok {
		// Drain anything sent before cancel, then expect the close.
		for range updates {
		}
	}
}

func TestSubscribeAfterExit(t *testing.T) {
	p, err := Start([]string{"echo", "bye"}, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wait(t, p.Done())
	snapshot, updates, cancel := p.Subscribe()
	defer cancel()
	if !strings.Contains(string(snapshot), "bye") {
		t.Errorf("snapshot = %q", snapshot)
	}
	if _, ok := <-updates; ok {
		t.Error("updates open after exit")
	}
}

func TestStopEndsProgramAndChildren(t *testing.T) {
	// The shell ignores SIGTERM, and its child sleeps. Stop must still end both.
	p, err := Start([]string{"sh", "-c", "trap '' TERM; sleep 60 & echo started; wait"}, t.TempDir(), []string{"PATH=" + os.Getenv("PATH")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, updates, cancel := p.Subscribe()
	defer cancel()
	for b := range updates {
		if bytes.Contains(b, []byte("started")) {
			break
		}
	}

	stopped := make(chan struct{})
	go func() {
		p.Stop(context.Background(), 100*time.Millisecond)
		close(stopped)
	}()
	wait(t, stopped)
	wait(t, p.Done())
	if p.ExitCode() == 0 {
		t.Error("exit code 0 after a kill")
	}

	// Stop after exit returns at once.
	p.Stop(context.Background(), time.Hour)
}

func TestShortProgramKeepsAllOutput(t *testing.T) {
	// A program that prints and exits at once must not lose its output.
	// macOS dropped it when the program's side of the terminal closed
	// before Tessaly had read it.
	for i := range 20 {
		p, err := Start([]string{"echo", "quick exit"}, t.TempDir(), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		wait(t, p.Done())
		if got := string(p.Output()); !strings.Contains(got, "quick exit") {
			t.Fatalf("run %d: output = %q", i, got)
		}
	}
}
