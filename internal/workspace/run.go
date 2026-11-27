// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tessaly-dev/tessaly/internal/ptyx"
	"github.com/tessaly-dev/tessaly/internal/store"
)

// StopGrace is how long an agent gets to exit after SIGTERM before SIGKILL.
const StopGrace = 10 * time.Second

// MaxRunning is how many agents may run at once. Each one is a process
// with up to ptyx.BufferSize of output in memory.
const MaxRunning = 16

// MaxTranscript is the largest a transcript file grows. A run that starts
// with a full transcript moves it to workspace-<id>.log.1, replacing the
// older one, and a run that fills it stops writing to it. The live terminal
// keeps working either way.
const MaxTranscript = 64 << 20

// ErrNotRunning means no agent runs in the workspace.
var ErrNotRunning = errors.New("no agent is running in this workspace")

// ErrTooManyRunning means MaxRunning agents run already.
var ErrTooManyRunning = fmt.Errorf("%d agents are running already. Stop one first", MaxRunning)

// Start runs argv as the workspace's agent, in its worktree, under a
// pseudo-terminal. Output is kept in memory and appended to the transcript
// at ~/.tessaly/logs/workspace-<id>.log. If argv holds PromptPath(ws.ID),
// Start writes prompt to that file first, and the file is deleted when the
// agent exits.
func (m *Manager) Start(ctx context.Context, ws store.Workspace, agentName string, argv []string, prompt string) (store.Workspace, error) {
	m.runMu.Lock()
	defer m.runMu.Unlock()

	ws, err := m.store.Workspace(ctx, ws.ID)
	if err != nil {
		return ws, err
	}
	if err := checkMove(State(ws.State), Running); err != nil {
		return ws, fmt.Errorf("%w: %w", ErrConflict, err)
	}
	if len(m.runs) >= m.maxRuns {
		return ws, ErrTooManyRunning
	}

	if slices.Contains(argv, m.PromptPath(ws.ID)) {
		if err := m.writePrompt(ws.ID, prompt); err != nil {
			return ws, err
		}
	}
	started := false
	defer func() {
		if !started {
			m.removePrompt(ws.ID)
		}
	}()

	transcript, err := m.openTranscript(ws.ID)
	if err != nil {
		return ws, err
	}
	header := fmt.Sprintf("\r\n--- tess: starting %s at %s ---\r\n", agentName, time.Now().UTC().Format(time.RFC3339))
	_, _ = transcript.Write([]byte(header))

	proc, err := ptyx.Start(argv, ws.Path, agentEnv(os.Environ(), ws), transcript)
	if err != nil {
		_ = transcript.Close()
		return ws, err
	}
	if err := m.store.SetWorkspaceRun(ctx, ws.ID, agentName, nil); err != nil {
		proc.Stop(ctx, 0)
		_ = transcript.Close()
		return ws, err
	}
	ws, err = m.move(ctx, ws, Running, "")
	if err != nil {
		proc.Stop(ctx, 0)
		_ = transcript.Close()
		return ws, err
	}
	ws.Agent, ws.ExitCode = agentName, nil

	started = true
	r := &run{proc: proc, recorded: make(chan struct{})}
	m.runs[ws.ID] = r
	// The agent outlives this request, so watch must not use its context.
	go m.watch(ws, r, transcript) // #nosec G118 -- see above.
	return ws, nil
}

// run is one agent process and a channel that closes once its exit is stored.
type run struct {
	proc     *ptyx.Process
	recorded chan struct{}
}

// watch waits for the agent to exit, then records how it ended.
func (m *Manager) watch(ws store.Workspace, r *run, transcript *cappedFile) {
	defer close(r.recorded)
	<-r.proc.Done()
	code := r.proc.ExitCode()
	_, _ = transcript.WriteNote(fmt.Sprintf("\r\n--- tess: exited with code %d ---\r\n", code))
	_ = transcript.Close()
	m.removePrompt(ws.ID)

	m.runMu.Lock()
	defer m.runMu.Unlock()
	delete(m.runs, ws.ID)
	// Use a fresh context: the request that started the agent is long gone.
	ctx := context.Background()
	if err := m.store.SetWorkspaceRun(ctx, ws.ID, ws.Agent, &code); err != nil {
		slog.Error("record agent exit", "workspace", ws.ID, "err", err)
	}
	ws.State = string(Running)
	if _, err := m.move(ctx, ws, Exited, ""); err != nil {
		slog.Error("record agent exit", "workspace", ws.ID, "err", err)
	}
}

// Stop ends the workspace's agent: SIGTERM, then SIGKILL after StopGrace.
// It returns once the agent has exited.
func (m *Manager) Stop(ctx context.Context, id int64) error {
	m.runMu.Lock()
	r := m.runs[id]
	m.runMu.Unlock()
	if r == nil {
		return ErrNotRunning
	}
	r.stop(ctx)
	return nil
}

// StopAll ends every running agent and waits for them. Call it on shutdown.
func (m *Manager) StopAll(ctx context.Context) {
	m.runMu.Lock()
	runs := make([]*run, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	m.runMu.Unlock()

	var wg sync.WaitGroup
	for _, r := range runs {
		wg.Go(func() { r.stop(ctx) })
	}
	wg.Wait()
}

// stop ends the process and waits until its exit is stored.
func (r *run) stop(ctx context.Context) {
	r.proc.Stop(ctx, StopGrace)
	select {
	case <-r.recorded:
	case <-ctx.Done():
	}
}

// Process returns the running agent of workspace id, or nil.
func (m *Manager) Process(id int64) *ptyx.Process {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	if r := m.runs[id]; r != nil {
		return r.proc
	}
	return nil
}

// TranscriptPath returns where the transcript of workspace id is kept.
func (m *Manager) TranscriptPath(id int64) string {
	return filepath.Join(m.home, "logs", "workspace-"+strconv.FormatInt(id, 10)+".log")
}

// PromptPath returns where the prompt of workspace id is written for an agent
// whose command holds "{prompt_file}".
func (m *Manager) PromptPath(id int64) string {
	return filepath.Join(m.home, "prompts", "workspace-"+strconv.FormatInt(id, 10)+".txt")
}

// writePrompt writes prompt to PromptPath(id) with mode 0600 in a 0700
// folder, refusing to follow a symlink there.
func (m *Manager) writePrompt(id int64, prompt string) error {
	path := m.PromptPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create prompts folder: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o600) // #nosec G304 -- path is built from the workspace id.
	if err != nil {
		return fmt.Errorf("write prompt file: %w", err)
	}
	if _, err := f.WriteString(prompt); err != nil {
		_ = f.Close()
		return fmt.Errorf("write prompt file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write prompt file: %w", err)
	}
	return nil
}

func (m *Manager) removePrompt(id int64) {
	if err := os.Remove(m.PromptPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Error("remove prompt file", "workspace", id, "err", err)
	}
}

// openTranscript opens the transcript of workspace id for appending. If it
// is full, it moves it to <path>.1 first and starts a new one.
func (m *Manager) openTranscript(id int64) (*cappedFile, error) {
	path := m.TranscriptPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create logs folder: %w", err)
	}
	if info, err := os.Lstat(path); err == nil && info.Size() >= m.maxTranscript {
		if err := os.Rename(path, path+".1"); err != nil {
			return nil, fmt.Errorf("rotate transcript: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) // #nosec G304 -- path is built from the workspace id.
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	return &cappedFile{f: f, size: info.Size(), max: m.maxTranscript}, nil
}

// cappedFile is a transcript that stops growing at max bytes. Writes past
// that are dropped, after one note that says so.
type cappedFile struct {
	f    *os.File
	size int64
	max  int64
	full bool
}

// fullNote ends a transcript that reached its size limit.
const fullNote = "\r\n--- tess: transcript full, later output is not saved ---\r\n"

// Write appends b up to the limit, then drops the rest. It reports all of b
// as written, so a full transcript never stops the agent's output.
func (c *cappedFile) Write(b []byte) (int, error) {
	if c.full {
		return len(b), nil
	}
	room := c.max - c.size
	if int64(len(b)) <= room {
		n, err := c.f.Write(b)
		c.size += int64(n)
		return n, err
	}
	c.full = true
	_, _ = c.f.Write(b[:max(room, 0)])
	_, _ = c.f.WriteString(fullNote)
	c.size = c.max
	return len(b), nil
}

// WriteNote appends a short Tessaly line even when the file is full, so
// the transcript always records how the run ended.
func (c *cappedFile) WriteNote(s string) (int, error) { return c.f.WriteString(s) }

func (c *cappedFile) Close() error { return c.f.Close() }

// agentEnv returns the environment for an agent: the daemon's own, without
// Tessaly and git variables, plus a terminal type and the workspace name.
func agentEnv(environ []string, ws store.Workspace) []string {
	env := make([]string, 0, len(environ)+3)
	for _, kv := range environ {
		if strings.HasPrefix(kv, "TESSALY_") || strings.HasPrefix(kv, "GIT_") || strings.HasPrefix(kv, "TERM=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"TERM=xterm-256color",
		"TESSALY_WORKSPACE="+ws.Slug,
		"TESSALY_BRANCH="+ws.Branch,
	)
}
