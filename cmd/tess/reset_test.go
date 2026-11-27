// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tessaly-dev/tessaly/internal/control"
	"github.com/tessaly-dev/tessaly/internal/store"
)

// prepare runs tess serve once so home holds a token and a database, then
// adds a passkey. It returns the token.
func prepare(t *testing.T, home string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if code := run(ctx, []string{"serve", "--port", "47779"}, &out, &out); code != 0 {
		t.Fatalf("serve = %d: %s", code, out.String())
	}
	addPasskey(t, home)
	return readToken(t, home)
}

func addPasskey(t *testing.T, home string) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(home, "tessaly.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.AddPasskey(context.Background(), []byte(time.Now().String()), "Laptop", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
}

func readToken(t *testing.T, home string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, "token"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(home, "token"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("token file = %v, %v, want mode 0600", info, err)
	}
	return strings.TrimSpace(string(b))
}

func passkeysAndEvents(t *testing.T, home string) (bool, []store.AuthEvent) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(home, "tessaly.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	has, err := db.HasPasskeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events, err := db.AuthEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return has, events
}

func TestAuthResetWhileStopped(t *testing.T) {
	home := testHome(t)
	t.Setenv("TESSALY_HOME", home)
	old := prepare(t, home)

	answer(t, hostname(t))
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"auth", "reset"}, &stdout, &stderr); code != 0 {
		t.Fatalf("auth reset = %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Passkeys removed: 1. Sessions ended: 0.") {
		t.Errorf("stdout = %q", stdout.String())
	}
	if got := readToken(t, home); got == old || len(got) != len(old) {
		t.Errorf("token %q after reset, want a new one", got)
	}
	has, events := passkeysAndEvents(t, home)
	if has {
		t.Error("passkeys remain after reset")
	}
	if len(events) != 1 || events[0].Method != "direct" || events[0].Passkeys != 1 {
		t.Errorf("audit log = %+v, want one direct reset of 1 passkey", events)
	}
}

func TestAuthResetWhileRunning(t *testing.T) {
	home := testHome(t)
	t.Setenv("TESSALY_HOME", home)
	old := prepare(t, home)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int)
	var serveOut bytes.Buffer
	go func() { done <- run(ctx, []string{"serve", "--port", "47779"}, &serveOut, &serveOut) }()
	defer func() {
		cancel()
		<-done
	}()
	waitFor(t, control.SocketPath(home))

	answer(t, hostname(t))
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"auth", "reset"}, &stdout, &stderr); code != 0 {
		t.Fatalf("auth reset = %d: %s", code, stderr.String())
	}
	token := readToken(t, home)
	if token == old {
		t.Error("token unchanged after reset")
	}
	// stdout is not a terminal, so the new token is not printed.
	if strings.Contains(stdout.String(), token) || !strings.Contains(stdout.String(), "Sign in with the token in") {
		t.Errorf("stdout = %q, want the token path and not the token", stdout.String())
	}
	has, events := passkeysAndEvents(t, home)
	if has || len(events) != 1 || events[0].Method != "socket" {
		t.Errorf("passkeys %v, audit log %+v, want none and one socket reset", has, events)
	}
}

func TestServeRefusesASecondDaemon(t *testing.T) {
	home := testHome(t)
	t.Setenv("TESSALY_HOME", home)
	prepare(t, home)
	release, err := control.Lock(home)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"serve", "--port", "47779"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "another tess serve") {
		t.Errorf("code %d, stderr %q, want 1 and a lock error", code, stderr.String())
	}
}

func TestAuthResetNeedsConfirmation(t *testing.T) {
	home := testHome(t)
	t.Setenv("TESSALY_HOME", home)
	old := prepare(t, home)
	answer(t, "reset")
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"auth", "reset"}, &stdout, &stderr); code != 1 {
		t.Errorf("reset with a wrong host name = %d, want 1", code)
	}
	if readToken(t, home) != old {
		t.Error("token changed without confirmation")
	}
	if code := run(context.Background(), []string{"auth", "reset", "--yes"}, &stdout, &stderr); code != 2 {
		t.Errorf("tess auth reset --yes = %d, want 2", code)
	}
	if code := run(context.Background(), []string{"auth"}, &stdout, &stderr); code != 2 {
		t.Errorf("tess auth = %d, want 2", code)
	}
}

// answer makes stdin hold line for the rest of the test.
func answer(t *testing.T, line string) {
	t.Helper()
	old := stdin
	stdin = strings.NewReader(line + "\n")
	t.Cleanup(func() { stdin = old })
}

func hostname(t *testing.T) string {
	t.Helper()
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	return host
}

func waitFor(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s did not appear", path)
}

func TestResetRefusesAgents(t *testing.T) {
	ran := false
	reset := func(context.Context) (any, error) { ran = true; return nil, nil }
	self := os.Getpid()
	h := resetHandler(func() []int { return []int{os.Getppid()} }, reset)
	var refused control.Refused
	if _, err := h(context.Background(), control.CmdAuthReset, self); !errors.As(err, &refused) || ran {
		t.Errorf("reset from under an agent: err %v, ran %v, want refused", err, ran)
	}
	h = resetHandler(func() []int { return nil }, reset)
	if _, err := h(context.Background(), control.CmdAuthReset, self); err != nil || !ran {
		t.Errorf("reset with no agents: err %v, ran %v, want it to run", err, ran)
	}
}
