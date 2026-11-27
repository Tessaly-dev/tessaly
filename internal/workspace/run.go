// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tessaly-dev/tessaly/internal/ptyx"
	"github.com/tessaly-dev/tessaly/internal/store"
)

// StopGrace is how long an agent gets to exit after SIGTERM before SIGKILL.
const StopGrace = 10 * time.Second

// ErrNotRunning means no agent runs in the workspace.
var ErrNotRunning = errors.New("no agent is running in this workspace")

// Start runs argv as the workspace's agent, in its worktree, under a
// pseudo-terminal. Output is kept in memory and appended to the transcript
// at ~/.tessaly/logs/workspace-<id>.log.
func (m *Manager) Start(ctx context.Context, ws store.Workspace, agentName string, argv []string) (store.Workspace, error) {
	m.runMu.Lock()
	defer m.runMu.Unlock()

	ws, err := m.store.Workspace(ctx, ws.ID)
	if err != nil {
		return ws, err
	}
	if err := checkMove(State(ws.State), Running); err != nil {
		return ws, fmt.Errorf("%w: %w", ErrConflict, err)
	}

	transcript, err := m.openTranscript(ws.ID)
	if err != nil {
		return ws, err
	}
	header := fmt.Sprintf("\r\n--- tess: starting %s at %s ---\r\n", agentName, time.Now().UTC().Format(time.RFC3339))
	_, _ = transcript.WriteString(header)

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
func (m *Manager) watch(ws store.Workspace, r *run, transcript *os.File) {
	defer close(r.recorded)
	<-r.proc.Done()
	code := r.proc.ExitCode()
	_, _ = fmt.Fprintf(transcript, "\r\n--- tess: exited with code %d ---\r\n", code)
	_ = transcript.Close()

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

func (m *Manager) openTranscript(id int64) (*os.File, error) {
	path := m.TranscriptPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create logs folder: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) // #nosec G304 -- path is built from the workspace id.
	if err != nil {
		return nil, fmt.Errorf("open transcript: %w", err)
	}
	return f, nil
}

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
