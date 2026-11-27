// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tessaly-dev/tessaly/internal/forge"
	"github.com/tessaly-dev/tessaly/internal/store"
)

// fakeForge records the pull requests it opens.
type fakeForge struct {
	opened []string
	err    error
}

func (f *fakeForge) OpenPR(_ context.Context, _, branch, title, body string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.opened = append(f.opened, branch+"|"+title+"|"+body)
	return "https://github.com/acme/app/pull/7", nil
}

func (f *fakeForge) Checks(context.Context, string, string) ([]forge.Check, error) {
	return nil, nil
}

// shipSetup returns a manager and a workspace whose repo has a bare
// origin remote and a git identity.
func shipSetup(t *testing.T) (*Manager, store.Workspace, string) {
	t.Helper()
	m, repo := setup(t)
	remote := t.TempDir()
	git(t, remote, "init", "-q", "--bare")
	git(t, repo.Path, "remote", "add", "origin", remote)
	git(t, repo.Path, "config", "user.name", "Test")
	git(t, repo.Path, "config", "user.email", "test@example.com")
	ws, err := m.Create(context.Background(), repo, "fix")
	if err != nil {
		t.Fatal(err)
	}
	return m, ws, remote
}

func TestShipCommitsPushesAndOpensPR(t *testing.T) {
	ctx := context.Background()
	m, ws, remote := shipSetup(t)
	f := &fakeForge{}

	if _, err := m.Ship(ctx, ws.ID, "Fix login", "", f); !errors.Is(err, ErrNothingToShip) {
		t.Fatalf("Ship(no changes) = %v, want ErrNothingToShip", err)
	}

	if err := os.WriteFile(filepath.Join(ws.Path, "login.go"), []byte("package login\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := m.Ship(ctx, ws.ID, "  Fix login  ", "Closes #1", f)
	if err != nil {
		t.Fatal(err)
	}
	if got.PRURL != "https://github.com/acme/app/pull/7" {
		t.Errorf("PRURL = %q", got.PRURL)
	}
	if len(f.opened) != 1 || f.opened[0] != "tess/fix|Fix login|Closes #1" {
		t.Errorf("opened = %q", f.opened)
	}
	if msg := git(t, ws.Path, "log", "-1", "--format=%s"); strings.TrimSpace(msg) != "Fix login" {
		t.Errorf("commit message = %q", msg)
	}
	if head, pushed := git(t, ws.Path, "rev-parse", "HEAD"), git(t, remote, "rev-parse", "tess/fix"); head != pushed {
		t.Errorf("remote has %s, want %s", pushed, head)
	}
	if stored, _ := m.store.Workspace(ctx, ws.ID); stored.PRURL != got.PRURL {
		t.Errorf("stored PRURL = %q", stored.PRURL)
	}

	// A second ship pushes new commits and does not open another pull request.
	if err := os.WriteFile(filepath.Join(ws.Path, "more.go"), []byte("package login\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Ship(ctx, ws.ID, "Add more", "", f); err != nil {
		t.Fatal(err)
	}
	if len(f.opened) != 1 {
		t.Errorf("opened %d pull requests, want 1", len(f.opened))
	}
	if head, pushed := git(t, ws.Path, "rev-parse", "HEAD"), git(t, remote, "rev-parse", "tess/fix"); head != pushed {
		t.Errorf("second push: remote has %s, want %s", pushed, head)
	}
}

func TestShipRefuses(t *testing.T) {
	ctx := context.Background()
	m, ws, _ := shipSetup(t)
	f := &fakeForge{}

	for _, title := range []string{"", "   ", "two\nlines", strings.Repeat("x", MaxTitle+1)} {
		if _, err := m.Ship(ctx, ws.ID, title, "", f); !errors.Is(err, ErrBadTitle) {
			t.Errorf("title %q: err = %v, want ErrBadTitle", title, err)
		}
	}
	if _, err := m.Ship(ctx, ws.ID, "t", strings.Repeat("x", MaxBody+1), f); !errors.Is(err, ErrBodyTooLong) {
		t.Errorf("long body: err = %v, want ErrBodyTooLong", err)
	}

	// A running agent, or another ship in progress, blocks shipping.
	if err := m.store.SetWorkspaceState(ctx, ws.ID, string(Running), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Ship(ctx, ws.ID, "t", "", f); !errors.Is(err, ErrConflict) {
		t.Errorf("running: err = %v, want ErrConflict", err)
	}
	if err := m.store.SetWorkspaceState(ctx, ws.ID, string(Exited), ""); err != nil {
		t.Fatal(err)
	}
	m.shipping[ws.ID] = true
	if _, err := m.Ship(ctx, ws.ID, "t", "", f); !errors.Is(err, ErrConflict) {
		t.Errorf("already shipping: err = %v, want ErrConflict", err)
	}
	repo, _ := m.store.Repo(ctx, ws.RepoID)
	if _, err := m.Archive(ctx, repo, ws); !errors.Is(err, ErrConflict) {
		t.Errorf("archive while shipping: err = %v, want ErrConflict", err)
	}
	delete(m.shipping, ws.ID)
}

func TestShipReportsFailedSteps(t *testing.T) {
	ctx := context.Background()
	m, ws, _ := shipSetup(t)
	if err := os.WriteFile(filepath.Join(ws.Path, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f := &fakeForge{err: errors.New("gh pr create: not signed in")}
	_, err := m.Ship(ctx, ws.ID, "t", "", f)
	var se *ShipError
	if !errors.As(err, &se) || se.Step != "open pull request" {
		t.Fatalf("err = %v, want a ShipError for the pull request step", err)
	}
	if m.shipping[ws.ID] {
		t.Error("workspace still marked as shipping after a failure")
	}

	git(t, ws.Path, "remote", "remove", "origin")
	if _, err := m.Ship(ctx, ws.ID, "t", "", &fakeForge{}); !errors.As(err, &se) || se.Step != "push" {
		t.Errorf("no remote: err = %v, want a ShipError for the push step", err)
	}
}
