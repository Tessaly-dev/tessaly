// SPDX-License-Identifier: Apache-2.0

// Package control is the daemon's host-only control channel. It is a Unix
// socket, ~/.tessaly/run/control.sock, apart from the HTTP server, so no web
// page or network client can reach it. The socket is 0600 inside a 0700
// folder, and the daemon also checks that the caller runs as the same OS
// user. It takes one command per connection, from a fixed list.
//
// The daemon also holds a lock file in the same folder while it runs, so a
// host command can tell whether to talk to the daemon or change the data
// folder directly.
package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// ErrLocked means another process, a running daemon, holds the lock.
var ErrLocked = errors.New("tess serve is running")

const (
	// CmdAuthReset ends every session, deletes every passkey and makes a
	// new login token.
	CmdAuthReset = "auth-reset"

	maxLine = 256
	// maxSocketPath fits the smallest Unix socket path limit, 104 bytes on
	// macOS, with room for the final NUL.
	maxSocketPath = 103
	ioTimeout     = 30 * time.Second
)

// Dir returns the control folder inside the data folder home.
func Dir(home string) string { return filepath.Join(home, "run") }

// SocketPath returns the control socket path inside home.
func SocketPath(home string) string { return filepath.Join(Dir(home), "control.sock") }

// ensureDir makes the control folder with mode 0700 and refuses one that
// other users can reach.
func ensureDir(home string) error {
	dir := Dir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s must be a folder with mode 0700", dir)
	}
	return nil
}

// Lock takes the daemon lock in home without waiting. It returns ErrLocked
// while another process holds it. The lock ends when release runs or the
// process exits, even by a crash.
func Lock(home string) (release func(), err error) {
	if err := ensureDir(home); err != nil {
		return nil, err
	}
	path := filepath.Join(Dir(home), "tess.lock")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|unix.O_NOFOLLOW, 0o600) // #nosec G304 -- the daemon's own lock file.
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil { // #nosec G115 -- a file descriptor fits in an int.
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return func() { _ = f.Close() }, nil
}

// Handler runs one command and returns the reply to send as JSON.
type Handler func(ctx context.Context, cmd string) (any, error)

// Listen opens the control socket in home. Take Lock first: the old socket
// file is removed, which is only safe while no other daemon runs.
func Listen(home string) (net.Listener, error) {
	if err := ensureDir(home); err != nil {
		return nil, err
	}
	path := SocketPath(home)
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf("control socket path %s is longer than %d bytes, which Unix sockets allow. "+
			"Set TESSALY_HOME to a shorter path", path, maxSocketPath)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove old control socket: %w", err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("control socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("control socket: %w", err)
	}
	return ln, nil
}

// reply is the one JSON line the daemon answers with.
type reply struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Serve answers commands on ln until ctx ends. Callers running as another
// OS user are refused.
func Serve(ctx context.Context, ln net.Listener, h Handler) {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("control socket stopped", "err", err)
			}
			return
		}
		go serveConn(ctx, conn, h)
	}
}

func serveConn(ctx context.Context, conn net.Conn, h Handler) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(ioTimeout))
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return
	}
	uid, err := peerUID(uc)
	if err != nil || uid != os.Getuid() {
		slog.Warn("control socket refused a caller", "uid", uid, "err", err)
		writeReply(conn, reply{Error: "refused: not the user running tess serve"})
		return
	}
	line, err := bufio.NewReader(&limitReader{conn, maxLine}).ReadString('\n')
	if err != nil {
		writeReply(conn, reply{Error: "bad command"})
		return
	}
	cmd := line[:len(line)-1]
	if cmd != CmdAuthReset {
		writeReply(conn, reply{Error: "unknown command"})
		return
	}
	v, err := h(ctx, cmd)
	if err != nil {
		slog.Error("control command failed", "cmd", cmd, "err", err)
		writeReply(conn, reply{Error: "command failed, see the tess serve log"})
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		writeReply(conn, reply{Error: "command failed"})
		return
	}
	writeReply(conn, reply{OK: true, Data: data})
}

func writeReply(conn net.Conn, r reply) {
	_ = json.NewEncoder(conn).Encode(r)
}

// limitReader reads at most n bytes, then reports EOF.
type limitReader struct {
	conn net.Conn
	n    int
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, errors.New("command too long")
	}
	if len(p) > l.n {
		p = p[:l.n]
	}
	n, err := l.conn.Read(p)
	l.n -= n
	return n, err
}

// Call sends cmd to the daemon's control socket in home and decodes the
// reply data into v.
func Call(home, cmd string, v any) error {
	conn, err := net.DialTimeout("unix", SocketPath(home), 5*time.Second)
	if err != nil {
		return fmt.Errorf("reach tess serve: %w", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(ioTimeout))
	if _, err := fmt.Fprintf(conn, "%s\n", cmd); err != nil {
		return fmt.Errorf("send to tess serve: %w", err)
	}
	var r reply
	if err := json.NewDecoder(conn).Decode(&r); err != nil {
		return fmt.Errorf("read from tess serve: %w", err)
	}
	if !r.OK {
		return fmt.Errorf("tess serve: %s", r.Error)
	}
	if v == nil {
		return nil
	}
	return json.Unmarshal(r.Data, v)
}
