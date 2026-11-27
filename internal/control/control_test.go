// SPDX-License-Identifier: Apache-2.0

package control

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// shortHome returns a data folder with a short path, because Unix socket
// paths are limited to about 100 bytes.
func shortHome(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "tess")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestLockIsExclusive(t *testing.T) {
	home := shortHome(t)
	release, err := Lock(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(home); !errors.Is(err, ErrLocked) {
		t.Errorf("second Lock = %v, want ErrLocked", err)
	}
	release()
	again, err := Lock(home)
	if err != nil {
		t.Fatalf("Lock after release = %v", err)
	}
	again()
	info, err := os.Stat(Dir(home))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("control folder = %v, %v, want mode 0700", info, err)
	}
}

func TestLockRefusesAnOpenFolder(t *testing.T) {
	home := shortHome(t)
	if err := os.Mkdir(Dir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(Dir(home), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(home); err == nil {
		t.Error("Lock accepted a folder other users can read")
	}
}

func serveTest(t *testing.T, h Handler) string {
	t.Helper()
	home := shortHome(t)
	ln, err := Listen(home)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(SocketPath(home))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("socket = %v, %v, want mode 0600", info, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Serve(ctx, ln, h)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return home
}

func TestCall(t *testing.T) {
	home := serveTest(t, func(_ context.Context, cmd string, pid int) (any, error) {
		if pid != os.Getpid() {
			t.Errorf("caller pid = %d, want %d", pid, os.Getpid())
		}
		return map[string]string{"ran": cmd}, nil
	})
	var got struct{ Ran string }
	if err := Call(home, CmdAuthReset, &got); err != nil {
		t.Fatal(err)
	}
	if got.Ran != CmdAuthReset {
		t.Errorf("ran %q, want %q", got.Ran, CmdAuthReset)
	}
	if err := Call(home, "rm -rf /", nil); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("unknown command = %v", err)
	}
}

func TestCallReportsFailureWithoutDetails(t *testing.T) {
	home := serveTest(t, func(context.Context, string, int) (any, error) {
		return nil, errors.New("secret detail")
	})
	err := Call(home, CmdAuthReset, nil)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Errorf("Call = %v, want a failure without the details", err)
	}
}

func TestCallPassesRefusals(t *testing.T) {
	home := serveTest(t, func(context.Context, string, int) (any, error) {
		return nil, Refused("not from an agent")
	})
	if err := Call(home, CmdAuthReset, nil); err == nil || !strings.Contains(err.Error(), "not from an agent") {
		t.Errorf("Call = %v, want the refusal", err)
	}
}

func TestServeRefusesLongCommands(t *testing.T) {
	home := serveTest(t, func(context.Context, string, int) (any, error) { return nil, nil })
	conn, err := net.Dial("unix", SocketPath(home))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(strings.Repeat("a", 4096))); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 512)
	n, _ := conn.Read(buf)
	if !strings.Contains(string(buf[:n]), "bad command") {
		t.Errorf("reply = %q, want bad command", buf[:n])
	}
}

func TestPeerUIDIsOurs(t *testing.T) {
	home := shortHome(t)
	ln, err := Listen(home)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := net.Dial("unix", SocketPath(home))
		if err == nil {
			defer func() { _ = c.Close() }()
			time.Sleep(100 * time.Millisecond)
		}
	}()
	conn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	uid, pid, err := peer(conn.(*net.UnixConn))
	if err != nil || uid != os.Getuid() || pid != os.Getpid() {
		t.Errorf("peer = %d, %d, %v, want %d, %d", uid, pid, err, os.Getuid(), os.Getpid())
	}
}
