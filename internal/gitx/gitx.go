// SPDX-License-Identifier: Apache-2.0

// Package gitx runs git commands. Every call uses a fixed argv with no shell,
// puts "--" before user values where git allows it, and turns off prompts.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// timeout caps every git call, so a hung git cannot hang a request.
const timeout = 30 * time.Second

// ErrNotRepo means the folder is not the top of a git work tree.
var ErrNotRepo = errors.New("not the top folder of a git repository")

// run runs git with args in dir and returns trimmed stdout.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- fixed argv, no shell.
	cmd.Dir = dir
	cmd.Env = append(env(os.Environ()),
		"GIT_TERMINAL_PROMPT=0", // Never wait for a password on a terminal.
		"GIT_OPTIONAL_LOCKS=0",  // Read commands must not take index locks.
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// RepoRoot checks that dir is the top folder of a non-bare git work tree and
// returns its absolute path with symlinks resolved. A subfolder of a repo, a
// bare repo or a plain folder returns ErrNotRepo. Errors name only dir, so
// callers can show them to the user.
func RepoRoot(ctx context.Context, dir string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("path must be absolute: %q", dir)
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a folder", real)
	}

	top, err := run(ctx, real, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotRepo
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil || top != real {
		return "", ErrNotRepo
	}
	return real, nil
}

// env returns environ without GIT_* variables. Variables such as GIT_DIR or
// GIT_WORK_TREE in the daemon's own environment would point git at another
// repo than the folder it runs in.
func env(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if !strings.HasPrefix(kv, "GIT_") {
			out = append(out, kv)
		}
	}
	return out
}

// Head returns the commit hash that HEAD points at in repo.
func Head(ctx context.Context, repo string) (string, error) {
	return run(ctx, repo, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
}

// BranchExists reports whether the local branch exists in repo.
func BranchExists(ctx context.Context, repo, branch string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// show-ref exits 1 when the ref is missing and 128 or more on real errors.
	cmd := exec.CommandContext(ctx, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch) // #nosec G204 -- fixed argv, no shell.
	cmd.Dir = repo
	cmd.Env = append(env(os.Environ()), "GIT_TERMINAL_PROMPT=0")
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false, nil
	default:
		return false, fmt.Errorf("git show-ref: %w", err)
	}
}

// AddWorktree creates a new branch at base and checks it out in a new
// worktree at path. path must not exist yet, or must be an empty folder.
func AddWorktree(ctx context.Context, repo, path, branch, base string) error {
	_, err := run(ctx, repo, "worktree", "add", "--quiet", "-b", branch, "--", path, base)
	return err
}

// ErrDirtyWorktree means the worktree has changes git would lose on removal.
var ErrDirtyWorktree = errors.New("the worktree has uncommitted or untracked files")

// RemoveWorktree removes the worktree at path and keeps its branch. It never
// forces: a worktree with uncommitted or untracked files returns
// ErrDirtyWorktree and stays as it is.
func RemoveWorktree(ctx context.Context, repo, path string) error {
	_, err := run(ctx, repo, "worktree", "remove", "--", path)
	if err != nil && strings.Contains(err.Error(), "modified or untracked files") {
		return ErrDirtyWorktree
	}
	return err
}

// HasWorktree reports whether path is one of repo's linked worktrees.
func HasWorktree(ctx context.Context, repo, path string) (bool, error) {
	out, err := run(ctx, repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return false, err
	}
	for _, field := range strings.Split(out, "\x00") {
		if p, ok := strings.CutPrefix(field, "worktree "); ok && p == path {
			return true, nil
		}
	}
	return false, nil
}
