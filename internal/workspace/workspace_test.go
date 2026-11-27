// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tessaly-dev/tessaly/internal/gitx"
	"github.com/tessaly-dev/tessaly/internal/store"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// setup returns a manager, and a stored repo with one commit.
func setup(t *testing.T) (*Manager, store.Repo) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(ctx, filepath.Join(dir, "tessaly.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	repoDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, repoDir, "init", "-q")
	git(t, repoDir, "commit", "-q", "--allow-empty", "-m", "first")

	repo, err := s.AddRepo(ctx, "app", repoDir)
	if err != nil {
		t.Fatal(err)
	}
	return NewManager(s, dir), repo
}

func TestTransitions(t *testing.T) {
	all := []State{Creating, Ready, Running, Exited, Archived, Failed}
	allowed := map[[2]State]bool{
		{Creating, Ready}:  true,
		{Creating, Failed}: true,
		{Ready, Running}:   true,
		{Ready, Archived}:  true,
		{Ready, Failed}:    true,
		{Running, Exited}:  true,
		{Exited, Running}:  true,
		{Exited, Archived}: true,
		{Failed, Archived}: true,
	}
	for _, from := range all {
		for _, to := range all {
			if got := CanMove(from, to); got != allowed[[2]State{from, to}] {
				t.Errorf("CanMove(%s, %s) = %v", from, to, got)
			}
		}
	}
}

func TestValidSlug(t *testing.T) {
	good := []string{"a", "fix-login", "0", "abc-123", "a23456789012345678901234567890123456789012345678"}
	bad := []string{"", "-a", "A", "fix login", "fix/login", "..", "a.b", "a_b", "a;rm", "a23456789012345678901234567890123456789012345678x"}
	for _, s := range good {
		if !ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = false", s)
		}
	}
	for _, s := range bad {
		if ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = true", s)
		}
	}
}

func TestCreateAndArchive(t *testing.T) {
	ctx := context.Background()
	m, repo := setup(t)

	ws, err := m.Create(ctx, repo, "fix-login")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ws.State != string(Ready) || ws.Branch != "tess/fix-login" {
		t.Errorf("ws = %+v, want ready on tess/fix-login", ws)
	}
	root, err := filepath.EvalSymlinks(m.root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "app", "fix-login"); ws.Path != want {
		t.Errorf("path = %q, want %q", ws.Path, want)
	}
	if got := git(t, ws.Path, "branch", "--show-current"); got != "tess/fix-login\n" {
		t.Errorf("worktree branch = %q", got)
	}
	info, err := os.Stat(filepath.Dir(ws.Path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("worktree parent mode = %04o, want 0700", perm)
	}

	ws, err = m.Archive(ctx, repo, ws)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if ws.State != string(Archived) {
		t.Errorf("state = %s, want archived", ws.State)
	}
	if _, err := os.Stat(ws.Path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("worktree still on disk: %v", err)
	}
	if ok, err := gitx.BranchExists(ctx, repo.Path, "tess/fix-login"); err != nil || !ok {
		t.Errorf("branch kept = %v, %v; want true", ok, err)
	}

	if _, err := m.Archive(ctx, repo, ws); !errors.Is(err, ErrConflict) {
		t.Errorf("archive twice: err = %v, want ErrConflict", err)
	}
}

func TestCreateRefuses(t *testing.T) {
	ctx := context.Background()
	m, repo := setup(t)

	if _, err := m.Create(ctx, repo, "Bad Name"); !errors.Is(err, ErrBadSlug) {
		t.Errorf("bad slug: err = %v", err)
	}

	git(t, repo.Path, "branch", "tess/taken")
	if _, err := m.Create(ctx, repo, "taken"); !errors.Is(err, ErrBranchExists) {
		t.Errorf("existing branch: err = %v", err)
	}

	if _, err := m.Create(ctx, repo, "once"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, repo, "once"); !errors.Is(err, ErrBranchExists) {
		t.Errorf("second create: err = %v, want ErrBranchExists", err)
	}
}

func TestCreateInEmptyRepo(t *testing.T) {
	ctx := context.Background()
	m, repo := setup(t)
	empty, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, empty, "init", "-q")
	repo.Path = empty

	if _, err := m.Create(ctx, repo, "first"); err == nil {
		t.Error("Create in a repo with no commits succeeded")
	}
}

func TestCreateMarksFailure(t *testing.T) {
	ctx := context.Background()
	m, repo := setup(t)

	// A file where the worktree should go makes git worktree add fail.
	target := filepath.Join(m.root, "app", "blocked")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	ws, err := m.Create(ctx, repo, "blocked")
	if err == nil {
		t.Fatal("Create succeeded")
	}
	if ws.State != string(Failed) || ws.Error == "" {
		t.Errorf("ws = %+v, want failed with an error", ws)
	}
	stored, err := m.store.Workspace(ctx, ws.ID)
	if err != nil || stored.State != string(Failed) {
		t.Errorf("stored = %+v, %v; want failed", stored, err)
	}

	// A failed workspace can be archived.
	if _, err := m.Archive(ctx, repo, ws); err != nil {
		t.Errorf("archive failed workspace: %v", err)
	}
}

func TestArchiveRefusesDirtyWorktree(t *testing.T) {
	ctx := context.Background()
	m, repo := setup(t)
	ws, err := m.Create(ctx, repo, "dirty")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, "new.txt"), []byte("work"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Archive(ctx, repo, ws); !errors.Is(err, gitx.ErrDirtyWorktree) {
		t.Fatalf("err = %v, want ErrDirtyWorktree", err)
	}
	if _, err := os.Stat(filepath.Join(ws.Path, "new.txt")); err != nil {
		t.Errorf("untracked file lost: %v", err)
	}
}

func TestWorktreePathStaysInRoot(t *testing.T) {
	m := NewManager(nil, "/data")
	for _, tt := range []struct{ repo, slug string }{{"..", "x"}, {"app", ".."}, {"../..", "etc"}, {"", ""}} {
		if p, err := m.worktreePath(tt.repo, tt.slug); err == nil {
			t.Errorf("worktreePath(%q, %q) = %q, want error", tt.repo, tt.slug, p)
		}
	}
	if p, err := m.worktreePath("app", "fix"); err != nil || p != "/data/worktrees/app/fix" {
		t.Errorf("worktreePath(app, fix) = %q, %v", p, err)
	}
}

func TestRecover(t *testing.T) {
	ctx := context.Background()
	m, repo := setup(t)
	stuck := store.Workspace{RepoID: repo.ID, Slug: "stuck", Branch: "tess/stuck", Base: "x", Path: "/nowhere/stuck", State: string(Creating)}
	if err := m.store.AddWorkspace(ctx, &stuck); err != nil {
		t.Fatal(err)
	}
	ready, err := m.Create(ctx, repo, "fine")
	if err != nil {
		t.Fatal(err)
	}
	running := store.Workspace{RepoID: repo.ID, Slug: "was-running", Branch: "tess/was-running", Base: "x", Path: "/nowhere/run", State: string(Running)}
	if err := m.store.AddWorkspace(ctx, &running); err != nil {
		t.Fatal(err)
	}

	if err := m.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := m.store.Workspace(ctx, stuck.ID)
	if got.State != string(Failed) || got.Error == "" {
		t.Errorf("stuck = %+v, want failed with a reason", got)
	}
	got, _ = m.store.Workspace(ctx, running.ID)
	if got.State != string(Exited) || got.Error == "" || got.Ended != EndInterrupted {
		t.Errorf("was-running = %+v, want exited, interrupted, with a reason", got)
	}
	got, _ = m.store.Workspace(ctx, ready.ID)
	if got.State != string(Ready) {
		t.Errorf("ready workspace changed to %s", got.State)
	}
}

// startAgent starts argv in a new ready workspace.
func startAgent(t *testing.T, m *Manager, repo store.Repo, slug string, argv ...string) store.Workspace {
	t.Helper()
	ws, err := m.Create(context.Background(), repo, slug)
	if err != nil {
		t.Fatal(err)
	}
	ws, err = m.Start(context.Background(), ws, "test", argv, "")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return ws
}

// waitExited waits until the agent's exit is stored.
func waitExited(t *testing.T, m *Manager, id int64) store.Workspace {
	t.Helper()
	m.runMu.Lock()
	r := m.runs[id]
	m.runMu.Unlock()
	if r != nil {
		select {
		case <-r.recorded:
		case <-time.After(10 * time.Second):
			t.Fatal("agent did not exit")
		}
	}
	ws, err := m.store.Workspace(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestStartRunsAgentInWorktree(t *testing.T) {
	m, repo := setup(t)
	t.Setenv("TESSALY_HOME", "/should/not/leak")
	t.Setenv("GIT_DIR", "/should/not/leak")
	ws := startAgent(t, m, repo, "run", "sh", "-c", `pwd; echo "ws=$TESSALY_WORKSPACE home=$TESSALY_HOME git=$GIT_DIR"; exit 7`)
	if ws.State != string(Running) || ws.Agent != "test" {
		t.Errorf("ws = %+v, want running with agent test", ws)
	}

	ws = waitExited(t, m, ws.ID)
	if ws.State != string(Exited) || ws.ExitCode == nil || *ws.ExitCode != 7 || ws.Ended != EndExited {
		t.Fatalf("ws = %+v, want exited on its own with code 7", ws)
	}

	transcript, err := os.ReadFile(m.TranscriptPath(ws.ID))
	if err != nil {
		t.Fatal(err)
	}
	out := string(transcript)
	for _, want := range []string{ws.Path, "ws=run home= git=", "starting test", "exited with code 7"} {
		if !strings.Contains(out, want) {
			t.Errorf("transcript lacks %q:\n%s", want, out)
		}
	}
	info, err := os.Stat(m.TranscriptPath(ws.ID))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("transcript mode = %04o, want 0600", perm)
	}

	// An exited workspace can run again, and then be archived.
	if _, err := m.Start(context.Background(), ws, "test", []string{"true"}, ""); err != nil {
		t.Fatalf("restart: %v", err)
	}
	ws = waitExited(t, m, ws.ID)
	if _, err := m.Archive(context.Background(), repo, ws); err != nil {
		t.Errorf("archive after exit: %v", err)
	}
}

func TestStartRefusesWhileRunning(t *testing.T) {
	m, repo := setup(t)
	ws := startAgent(t, m, repo, "busy", "sleep", "60")
	defer m.StopAll(context.Background())

	if _, err := m.Start(context.Background(), ws, "test", []string{"true"}, ""); !errors.Is(err, ErrConflict) {
		t.Errorf("second start: err = %v, want ErrConflict", err)
	}
	if _, err := m.Archive(context.Background(), repo, ws); !errors.Is(err, ErrConflict) {
		t.Errorf("archive while running: err = %v, want ErrConflict", err)
	}
}

func TestStartMissingProgramKeepsState(t *testing.T) {
	m, repo := setup(t)
	ws, err := m.Create(context.Background(), repo, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), ws, "test", []string{"/no/such/agent"}, ""); err == nil {
		t.Fatal("Start of a missing program succeeded")
	}
	got, _ := m.store.Workspace(context.Background(), ws.ID)
	if got.State != string(Ready) {
		t.Errorf("state = %s, want ready", got.State)
	}
}

func TestStopAndStopAll(t *testing.T) {
	m, repo := setup(t)
	a := startAgent(t, m, repo, "a", "sleep", "60")
	b := startAgent(t, m, repo, "b", "sleep", "60")

	if err := m.Stop(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	if got := waitExited(t, m, a.ID); got.State != string(Exited) || got.Ended != EndStopped {
		t.Errorf("a = %+v, want exited and stopped", got)
	}
	if err := m.Stop(context.Background(), a.ID); !errors.Is(err, ErrNotRunning) {
		t.Errorf("stop twice: err = %v, want ErrNotRunning", err)
	}

	m.StopAll(context.Background())
	if got := waitExited(t, m, b.ID); got.State != string(Exited) || got.Ended != EndInterrupted {
		t.Errorf("b = %+v, want exited and interrupted", got)
	}
	if m.Process(b.ID) != nil {
		t.Error("process still tracked after StopAll")
	}
}

func TestResume(t *testing.T) {
	ctx := context.Background()
	m, repo := setup(t)
	ws := startAgent(t, m, repo, "cut", "sleep", "60")

	if _, err := m.Resume(ctx, ws.ID, []string{"true"}); !errors.Is(err, ErrConflict) {
		t.Errorf("resume while running: err = %v, want ErrConflict", err)
	}
	m.StopAll(ctx)
	waitExited(t, m, ws.ID)

	got, err := m.Resume(ctx, ws.ID, []string{"sh", "-c", "echo picked up"})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got.State != string(Running) || got.Agent != "test" || got.Ended != "" {
		t.Errorf("resumed = %+v, want running as the same agent", got)
	}
	got = waitExited(t, m, ws.ID)
	if got.Ended != EndExited {
		t.Errorf("after resume ended = %q, want %q", got.Ended, EndExited)
	}
	transcript, _ := os.ReadFile(m.TranscriptPath(ws.ID))
	if !strings.Contains(string(transcript), "picked up") {
		t.Errorf("transcript lacks the resumed run:\n%s", transcript)
	}

	// A run that ended on its own does not resume.
	if _, err := m.Resume(ctx, ws.ID, []string{"true"}); !errors.Is(err, ErrConflict) {
		t.Errorf("resume after a normal exit: err = %v, want ErrConflict", err)
	}
}

func TestWaiting(t *testing.T) {
	m, repo := setup(t)
	m.IdleAfter = 200 * time.Millisecond
	quiet := startAgent(t, m, repo, "quiet", "sleep", "60")
	busy := startAgent(t, m, repo, "busy", "sh", "-c", "while :; do echo working; sleep 0.05; done")
	defer m.StopAll(context.Background())

	if m.Waiting(quiet.ID) {
		t.Error("a just-started agent counts as waiting")
	}
	time.Sleep(400 * time.Millisecond) // Lets IdleAfter pass for the real process.
	if !m.Waiting(quiet.ID) {
		t.Error("a quiet agent does not count as waiting")
	}
	if m.Waiting(busy.ID) {
		t.Error("an agent that keeps printing counts as waiting")
	}
	if m.Waiting(0) {
		t.Error("a workspace with no agent counts as waiting")
	}
}

func TestAgentEnv(t *testing.T) {
	ws := store.Workspace{Slug: "fix", Branch: "tess/fix"}
	got := agentEnv([]string{"HOME=/h", "TESSALY_HOME=/t", "GIT_DIR=/g", "TERM=dumb", "ANTHROPIC_API_KEY=k"}, ws)
	want := []string{"HOME=/h", "ANTHROPIC_API_KEY=k", "TERM=xterm-256color", "TESSALY_WORKSPACE=fix", "TESSALY_BRANCH=tess/fix"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("agentEnv = %v, want %v", got, want)
	}
}

func TestStartCapsRunningAgents(t *testing.T) {
	m, repo := setup(t)
	m.maxRuns = 1
	startAgent(t, m, repo, "first", "sleep", "60")
	defer m.StopAll(context.Background())

	ws, err := m.Create(context.Background(), repo, "second")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), ws, "test", []string{"true"}, ""); !errors.Is(err, ErrTooManyRunning) {
		t.Fatalf("start over the cap: err = %v, want ErrTooManyRunning", err)
	}
	if got, _ := m.store.Workspace(context.Background(), ws.ID); got.State != string(Ready) {
		t.Errorf("state = %s, want ready", got.State)
	}
}

func TestTranscriptStopsAtLimitAndRotates(t *testing.T) {
	m, repo := setup(t)
	m.maxTranscript = 200
	ws := startAgent(t, m, repo, "chatty", "sh", "-c", "i=0; while [ $i -lt 50 ]; do echo line-$i; i=$((i+1)); done")
	ws = waitExited(t, m, ws.ID)

	path := m.TranscriptPath(ws.ID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "transcript full") || !strings.Contains(out, "exited with code 0") {
		t.Errorf("transcript lacks the full note or the exit line:\n%s", out)
	}
	if strings.Contains(out, "line-49") || !strings.Contains(out, "line-0") {
		t.Error("transcript should keep early output and drop output past its limit")
	}
	if limit := 200 + len(fullNote) + 64; len(data) > limit {
		t.Errorf("transcript is %d bytes, want at most %d", len(data), limit)
	}

	// The next run moves the full transcript aside and starts a new one.
	if _, err := m.Start(context.Background(), ws, "test", []string{"echo", "second run"}, ""); err != nil {
		t.Fatal(err)
	}
	waitExited(t, m, ws.ID)
	old, err := os.ReadFile(path + ".1")
	if err != nil || string(old) != out {
		t.Errorf("rotated transcript = %q, %v, want the first run", old, err)
	}
	if fresh, _ := os.ReadFile(path); !strings.Contains(string(fresh), "second run") || strings.Contains(string(fresh), "line-0") {
		t.Errorf("new transcript = %q", fresh)
	}
	if info, err := os.Stat(path + ".1"); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("rotated transcript mode = %v, %v, want 0600", info, err)
	}
}

func TestStartWritesPromptFileAndRemovesIt(t *testing.T) {
	m, repo := setup(t)
	ws, err := m.Create(context.Background(), repo, "secret")
	if err != nil {
		t.Fatal(err)
	}
	path := m.PromptPath(ws.ID)
	ws, err = m.Start(context.Background(), ws, "test", []string{"sh", "-c", `ls -l "$1"; cat "$1"`, "x", path}, "the secret task")
	if err != nil {
		t.Fatal(err)
	}
	ws = waitExited(t, m, ws.ID)
	out, err := os.ReadFile(m.TranscriptPath(ws.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "-rw-------") || !strings.Contains(string(out), "the secret task") {
		t.Errorf("agent did not read a 0600 prompt file:\n%s", out)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("prompt file still there after exit: %v", err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("prompts folder = %v, %v, want mode 0700", info, err)
	}

	// A start that fails leaves no prompt file behind.
	if _, err := m.Start(context.Background(), ws, "test", []string{"/no/such/agent", path}, "the secret task"); err == nil {
		t.Fatal("start of a missing program succeeded")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("prompt file left after a failed start: %v", err)
	}
}

func TestStartLeavesARunningAgentsPromptAlone(t *testing.T) {
	m, repo := setup(t)
	ws, err := m.Create(context.Background(), repo, "busy")
	if err != nil {
		t.Fatal(err)
	}
	path := m.PromptPath(ws.ID)
	ws, err = m.Start(context.Background(), ws, "test", []string{"sh", "-c", "sleep 60", "x", path}, "first")
	if err != nil {
		t.Fatal(err)
	}
	defer m.StopAll(context.Background())
	if _, err := m.Start(context.Background(), ws, "test", []string{"true", path}, "second"); !errors.Is(err, ErrConflict) {
		t.Fatalf("second start: err = %v, want ErrConflict", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "first" {
		t.Errorf("prompt file = %q, %v, want the running agent's", got, err)
	}
}
