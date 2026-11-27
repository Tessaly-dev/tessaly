// SPDX-License-Identifier: Apache-2.0

package gitx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitIn runs git in dir with a fixed author and fails the test on error.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=T", "-c", "user.email=t@example.com"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// diffRepo makes a repo with one committed file and returns it and its base.
func diffRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := gitInit(t)
	write(t, filepath.Join(repo, "keep.txt"), "one\n")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "first")
	base, err := Head(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return repo, base
}

func TestDiffShowsCommittedEditedAndNewFiles(t *testing.T) {
	ctx := context.Background()
	repo, base := diffRepo(t)

	write(t, filepath.Join(repo, "committed.txt"), "from a commit\n")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-q", "-m", "second")
	write(t, filepath.Join(repo, "keep.txt"), "one\ntwo\n")
	write(t, filepath.Join(repo, "new file.txt"), "brand new\n")
	write(t, filepath.Join(repo, ".gitignore"), "ignored.txt\n")
	write(t, filepath.Join(repo, "ignored.txt"), "secret\n")

	patch, truncated, err := Diff(ctx, repo, base, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Error("truncated = true, want false")
	}
	for _, want := range []string{"+from a commit", "+two", "+++ b/new file.txt", "+brand new", "+ignored.txt"} {
		if !strings.Contains(patch, want) {
			t.Errorf("patch lacks %q:\n%s", want, patch)
		}
	}
	if strings.Contains(patch, "secret") {
		t.Errorf("patch shows an ignored file:\n%s", patch)
	}
}

func TestDiffCleanTreeIsEmpty(t *testing.T) {
	repo, base := diffRepo(t)
	patch, truncated, err := Diff(context.Background(), repo, base, 1<<20)
	if err != nil || patch != "" || truncated {
		t.Errorf("Diff = %q, %v, %v; want empty", patch, truncated, err)
	}
}

func TestDiffStopsAtLimit(t *testing.T) {
	repo, base := diffRepo(t)
	write(t, filepath.Join(repo, "big.txt"), strings.Repeat("line\n", 1000))
	patch, truncated, err := Diff(context.Background(), repo, base, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(patch) != 100 {
		t.Errorf("len = %d, truncated = %v; want 100, true", len(patch), truncated)
	}
}

func TestDiffIgnoresExternalDiffConfig(t *testing.T) {
	repo, base := diffRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	gitIn(t, repo, "config", "diff.external", "touch "+marker)
	write(t, filepath.Join(repo, "keep.txt"), "changed\n")
	if _, _, err := Diff(context.Background(), repo, base, 1<<20); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("Diff ran the external diff program from repo config")
	}
}

func TestDiffRejectsBadBase(t *testing.T) {
	repo, _ := diffRepo(t)
	for _, base := range []string{"", "HEAD", "--output=/tmp/x", "main"} {
		if _, _, err := Diff(context.Background(), repo, base, 1<<20); err == nil {
			t.Errorf("Diff(base %q) = nil error", base)
		}
	}
}

func TestDiffCapsNewFiles(t *testing.T) {
	repo, base := diffRepo(t)
	for i := range maxUntracked + 1 {
		write(t, filepath.Join(repo, fmt.Sprintf("f%03d.txt", i)), "x\n")
	}
	patch, truncated, err := Diff(context.Background(), repo, base, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(patch, "diff --git "); !truncated || n != maxUntracked {
		t.Errorf("files = %d, truncated = %v; want %d, true", n, truncated, maxUntracked)
	}
}
