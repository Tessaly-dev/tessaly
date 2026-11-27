// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
	return NewManager(s, filepath.Join(dir, "worktrees")), repo
}

func TestTransitions(t *testing.T) {
	all := []State{Creating, Ready, Archived, Failed}
	allowed := map[[2]State]bool{
		{Creating, Ready}:  true,
		{Creating, Failed}: true,
		{Ready, Archived}:  true,
		{Ready, Failed}:    true,
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
	m := NewManager(nil, "/data/worktrees")
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

	if err := m.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := m.store.Workspace(ctx, stuck.ID)
	if got.State != string(Failed) || got.Error == "" {
		t.Errorf("stuck = %+v, want failed with a reason", got)
	}
	got, _ = m.store.Workspace(ctx, ready.ID)
	if got.State != string(Ready) {
		t.Errorf("ready workspace changed to %s", got.State)
	}
}
