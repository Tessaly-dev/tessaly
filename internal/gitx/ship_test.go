// SPDX-License-Identifier: Apache-2.0

package gitx

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestCommitAllAndPush(t *testing.T) {
	ctx := context.Background()
	repo, base := diffRepo(t)
	gitIn(t, repo, "config", "user.name", "T")
	gitIn(t, repo, "config", "user.email", "t@example.com")
	remote := gitInit(t, "--bare")
	gitIn(t, repo, "remote", "add", "origin", remote)
	gitIn(t, repo, "switch", "-q", "-c", "tess/fix")

	if dirty, err := Dirty(ctx, repo); err != nil || dirty {
		t.Fatalf("Dirty(clean) = %v, %v", dirty, err)
	}
	write(t, filepath.Join(repo, "new.txt"), "hi\n")
	if dirty, err := Dirty(ctx, repo); err != nil || !dirty {
		t.Fatalf("Dirty(new file) = %v, %v", dirty, err)
	}

	// A message that looks like a flag is still just the message.
	if err := CommitAll(ctx, repo, "--amend\n\nBody line"); err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, repo, "log", "-1", "--format=%B"); got != "--amend\n\nBody line" {
		t.Errorf("message = %q", got)
	}
	if got := gitOut(t, repo, "rev-list", "--count", "HEAD"); got != "2" {
		t.Errorf("commits = %s, want 2 (no amend)", got)
	}
	if dirty, _ := Dirty(ctx, repo); dirty {
		t.Error("tree still dirty after CommitAll")
	}

	if n, err := Ahead(ctx, repo, base, "tess/fix"); err != nil || n != 1 {
		t.Errorf("Ahead = %d, %v; want 1", n, err)
	}
	if _, err := Ahead(ctx, repo, "HEAD", "tess/fix"); err == nil {
		t.Error("Ahead accepted a base that is not a hash")
	}

	if err := Push(ctx, repo, "origin", "tess/fix"); err != nil {
		t.Fatal(err)
	}
	if got, want := gitOut(t, remote, "rev-parse", "refs/heads/tess/fix"), gitOut(t, repo, "rev-parse", "HEAD"); got != want {
		t.Errorf("remote branch = %s, want %s", got, want)
	}
	if got := gitOut(t, repo, "rev-parse", "--abbrev-ref", "tess/fix@{upstream}"); got != "origin/tess/fix" {
		t.Errorf("upstream = %s", got)
	}
}

func TestPushNeverForces(t *testing.T) {
	ctx := context.Background()
	repo, _ := diffRepo(t)
	remote := gitInit(t, "--bare")
	gitIn(t, repo, "remote", "add", "origin", remote)
	gitIn(t, repo, "switch", "-q", "-c", "tess/fix")
	write(t, filepath.Join(repo, "a.txt"), "a\n")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "a")
	if err := Push(ctx, repo, "origin", "tess/fix"); err != nil {
		t.Fatal(err)
	}
	// Rewrite history locally. The push must be refused.
	gitIn(t, repo, "commit", "-q", "--amend", "-m", "rewritten")
	if err := Push(ctx, repo, "origin", "tess/fix"); err == nil {
		t.Error("Push of rewritten history succeeded, want refusal")
	}
}

func TestPushErrorHidesCredentials(t *testing.T) {
	repo, _ := diffRepo(t)
	gitIn(t, repo, "remote", "add", "origin", "https://me:ghp_secret@127.0.0.1:1/x.git")
	err := Push(context.Background(), repo, "origin", gitOut(t, repo, "branch", "--show-current"))
	if err == nil {
		t.Fatal("Push to a dead remote succeeded")
	}
	if !strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("error does not come from the push: %v", err)
	}
	if strings.Contains(err.Error(), "ghp_secret") {
		t.Errorf("error leaks the token: %v", err)
	}
}

func TestRedact(t *testing.T) {
	got := Redact("fatal: unable to access 'https://x-access-token:abc@github.com/a/b.git/'")
	if strings.Contains(got, "abc") || !strings.Contains(got, "https://***@github.com") {
		t.Errorf("Redact = %q", got)
	}
}
