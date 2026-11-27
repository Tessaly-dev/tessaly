// SPDX-License-Identifier: Apache-2.0

// Package ptyx runs a program under a pseudo-terminal, keeps the tail of its
// output and streams it to subscribers.
package ptyx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// BufferSize is how much recent output each process keeps in memory.
const BufferSize = 1 << 20

// drainTimeout is how long to keep reading output after the program exits,
// in case a child it left behind still holds the terminal open.
const drainTimeout = 2 * time.Second

// quietPeriod is how long the terminal must stay silent after the program
// exits before Tessaly closes it.
const quietPeriod = 100 * time.Millisecond

// Process is a program running under a pseudo-terminal.
type Process struct {
	cmd *exec.Cmd
	tty *os.File // the controlling side, which Tessaly reads and writes
	// slave is the program's side. Tessaly holds it open until the output
	// is read, because macOS drops unread output when the last holder of
	// the program's side closes it.
	slave      *os.File
	transcript io.Writer
	done       chan struct{}

	mu       sync.Mutex
	out      *ring
	subs     map[chan []byte]struct{}
	exitCode int
}

// Start runs argv in dir with env under a new pseudo-terminal. The program
// gets its own session, so Stop can signal it and every child it started.
// Output also goes to transcript, if it is not nil.
func Start(argv []string, dir string, env []string, transcript io.Writer) (*Process, error) {
	if len(argv) == 0 {
		return nil, errors.New("empty command")
	}
	cmd := exec.Command(argv[0], argv[1:]...) // #nosec G204 -- argv comes from the user's own config, and no shell is involved.
	cmd.Dir = dir
	cmd.Env = env
	tty, slave, err := pty.Open()
	if err != nil {
		return nil, fmt.Errorf("open terminal: %w", err)
	}
	if err := pty.Setsize(tty, &pty.Winsize{Rows: 30, Cols: 120}); err != nil {
		_, _ = tty.Close(), slave.Close()
		return nil, fmt.Errorf("size terminal: %w", err)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		_, _ = tty.Close(), slave.Close()
		return nil, fmt.Errorf("start %s: %w", argv[0], err)
	}

	p := &Process{
		cmd:        cmd,
		tty:        tty,
		slave:      slave,
		transcript: transcript,
		done:       make(chan struct{}),
		out:        newRing(BufferSize),
		subs:       make(map[chan []byte]struct{}),
	}
	go p.run()
	return p, nil
}

// run copies output until the program exits, then records the exit code.
func (p *Process) run() {
	copied := make(chan struct{})
	read := make(chan struct{}, 1)
	go func() {
		defer close(copied)
		buf := make([]byte, 32<<10)
		for {
			n, err := p.tty.Read(buf)
			if n > 0 {
				p.publish(buf[:n])
				select {
				case read <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return // EOF, or EIO on Linux once the program has exited.
			}
		}
	}()

	err := p.cmd.Wait()
	p.drain(read, copied)
	_ = p.slave.Close()
	select {
	case <-copied:
	case <-time.After(quietPeriod):
	}
	_ = p.tty.Close()
	<-copied

	code := 0
	if err != nil {
		code = -1
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		}
	}

	p.mu.Lock()
	p.exitCode = code
	for ch := range p.subs {
		close(ch)
		delete(p.subs, ch)
	}
	p.mu.Unlock()
	close(p.done)
}

// drain waits, after the program exits, until its output goes quiet for
// quietPeriod, or for drainTimeout at most if a child it left behind keeps
// writing.
func (p *Process) drain(read, copied <-chan struct{}) {
	quiet := time.NewTimer(quietPeriod)
	defer quiet.Stop()
	limit := time.NewTimer(drainTimeout)
	defer limit.Stop()
	for {
		select {
		case <-read:
			quiet.Reset(quietPeriod)
		case <-quiet.C:
			return
		case <-limit.C:
			return
		case <-copied:
			return
		}
	}
}

func (p *Process) publish(b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.out.Write(b)
	if p.transcript != nil {
		_, _ = p.transcript.Write(b)
	}
	for ch := range p.subs {
		select {
		case ch <- append([]byte(nil), b...):
		default:
			// A subscriber that cannot keep up is dropped rather than
			// slowing the program. It can subscribe again for a snapshot.
			close(ch)
			delete(p.subs, ch)
		}
	}
}

// Subscribe returns the output so far and a channel of new output. The
// channel closes when the program exits, when cancel is called, or when the
// subscriber falls too far behind.
func (p *Process) Subscribe() (snapshot []byte, updates <-chan []byte, cancel func()) {
	ch := make(chan []byte, 256)
	p.mu.Lock()
	defer p.mu.Unlock()
	snapshot = p.out.Bytes()
	select {
	case <-p.done:
		close(ch)
		return snapshot, ch, func() {}
	default:
	}
	p.subs[ch] = struct{}{}
	return snapshot, ch, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if _, ok := p.subs[ch]; ok {
			close(ch)
			delete(p.subs, ch)
		}
	}
}

// Output returns a copy of the most recent output, up to BufferSize bytes.
func (p *Process) Output() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.out.Bytes()
}

// Write sends input to the program as if typed.
func (p *Process) Write(b []byte) (int, error) { return p.tty.Write(b) }

// Resize sets the terminal size.
func (p *Process) Resize(rows, cols uint16) error {
	return pty.Setsize(p.tty, &pty.Winsize{Rows: rows, Cols: cols})
}

// Done closes when the program has exited and its output is read.
func (p *Process) Done() <-chan struct{} { return p.done }

// ExitCode returns the exit code once Done is closed: -1 if a signal ended
// the program.
func (p *Process) ExitCode() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitCode
}

// Stop asks the program and its children to stop with SIGTERM, and sends
// SIGKILL if they are still running after grace or when ctx ends. It
// returns once the program has exited.
func (p *Process) Stop(ctx context.Context, grace time.Duration) {
	select {
	case <-p.done:
		return // Never signal a process group that may have been reused.
	default:
	}
	pgid := p.cmd.Process.Pid // The program leads its own session and group.
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-p.done:
		return
	case <-timer.C:
	case <-ctx.Done():
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	<-p.done
}
