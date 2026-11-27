// SPDX-License-Identifier: Apache-2.0

package gitx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitInit makes a new repo in a temp folder and returns its resolved path.
func gitInit(t *testing.T, args ...string) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", append([]string{"init", "-q"}, append(args, dir)...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestRepoRoot(t *testing.T) {
	ctx := context.Background()
	repo := gitInit(t)

	got, err := RepoRoot(ctx, repo)
	if err != nil || got != repo {
		t.Fatalf("RepoRoot(repo) = %q, %v; want %q", got, err, repo)
	}

	// A trailing slash and "." segments are cleaned.
	if got, err := RepoRoot(ctx, repo+"/./"); err != nil || got != repo {
		t.Errorf("RepoRoot(repo/./) = %q, %v; want %q", got, err, repo)
	}

	// A symlink to the repo resolves to the real path.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	if got, err := RepoRoot(ctx, link); err != nil || got != repo {
		t.Errorf("RepoRoot(link) = %q, %v; want %q", got, err, repo)
	}
}

func TestRepoRootRejects(t *testing.T) {
	ctx := context.Background()
	repo := gitInit(t)
	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(repo, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		path   string
		notGit bool
	}{
		{"subfolder of a repo", sub, true},
		{"bare repo", gitInit(t, "--bare"), true},
		{"plain folder", t.TempDir(), true},
		{"relative path", "relative/repo", false},
		{"missing path", filepath.Join(repo, "missing"), false},
		{"file", file, false},
		{"flag-like path", "--help", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RepoRoot(ctx, tt.path)
			if err == nil {
				t.Fatal("RepoRoot accepted it")
			}
			if tt.notGit && !errors.Is(err, ErrNotRepo) {
				t.Errorf("err = %v, want ErrNotRepo", err)
			}
		})
	}
}

func TestRepoRootIgnoresGitEnv(t *testing.T) {
	repo := gitInit(t)
	other := gitInit(t)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)

	got, err := RepoRoot(context.Background(), repo)
	if err != nil || got != repo {
		t.Errorf("RepoRoot = %q, %v; want %q", got, err, repo)
	}
}

func TestEnv(t *testing.T) {
	got := env([]string{"HOME=/h", "GIT_DIR=/x", "PATH=/bin", "GIT_CONFIG_GLOBAL=/y"})
	if len(got) != 2 || got[0] != "HOME=/h" || got[1] != "PATH=/bin" {
		t.Errorf("env() = %v, want HOME and PATH only", got)
	}
}
