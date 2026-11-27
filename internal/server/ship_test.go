// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tessaly-dev/tessaly/internal/store"
)

type fakeForge struct{ err error }

func (f fakeForge) OpenPR(context.Context, string, string, string, string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "https://github.com/acme/app/pull/7", nil
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// shipWorkspace makes a workspace with a changed file in a repo that has a
// bare origin and a git identity, and returns its ship URL.
func shipWorkspace(t *testing.T, f fakeForge) (http.Handler, *creds, string) {
	t.Helper()
	cfg := testConfig(t, testPort, testUI)
	cfg.Forge = f
	h := Handler(cfg)
	c := signIn(t, h)
	repo := addRepoWithCommit(t, h, c)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, repo.Path, "init", "-q", "--bare", remote)
	gitRun(t, repo.Path, "remote", "add", "origin", remote)
	gitRun(t, repo.Path, "config", "user.name", "T")
	gitRun(t, repo.Path, "config", "user.email", "t@example.com")

	rec := send(t, h, http.MethodPost, "/api/v1/workspaces", wsBody(repo.ID, "fix-login"), c, nil)
	var ws store.Workspace
	if err := json.NewDecoder(rec.Body).Decode(&ws); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, "login.go"), []byte("package login\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return h, c, "/api/v1/workspaces/" + strconv.FormatInt(ws.ID, 10) + "/ship"
}

func shipBody(title, body string) string {
	b, _ := json.Marshal(map[string]string{"title": title, "body": body})
	return string(b)
}

func TestShipAPI(t *testing.T) {
	h, c, url := shipWorkspace(t, fakeForge{})

	if rec := send(t, h, http.MethodPost, url, shipBody("Fix login", ""), nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out = %d, want 401", rec.Code)
	}
	if rec := send(t, h, http.MethodPost, url, shipBody("", ""), c, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("empty title = %d, want 400", rec.Code)
	}
	if rec := send(t, h, http.MethodPost, url, `{"title":"x","base":"main"}`, c, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field = %d, want 400", rec.Code)
	}
	if rec := send(t, h, http.MethodPost, "/api/v1/workspaces/999/ship", shipBody("x", ""), c, nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing workspace = %d, want 404", rec.Code)
	}

	// A body near the limit, full of characters JSON escapes, still fits.
	rec := send(t, h, http.MethodPost, url, shipBody("Fix login", strings.Repeat("\"", 60_000)), c, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("ship = %d: %s", rec.Code, rec.Body)
	}
	var ws store.Workspace
	if err := json.NewDecoder(rec.Body).Decode(&ws); err != nil {
		t.Fatal(err)
	}
	if ws.PRURL != "https://github.com/acme/app/pull/7" {
		t.Errorf("prUrl = %q", ws.PRURL)
	}
	// Shipping again with no new commits is a harmless push.
	if rec := send(t, h, http.MethodPost, url, shipBody("Again", ""), c, nil); rec.Code != http.StatusOK {
		t.Errorf("ship again = %d, want 200: %s", rec.Code, rec.Body)
	}
}

func TestShipAPIShowsStepErrors(t *testing.T) {
	h, c, url := shipWorkspace(t, fakeForge{err: errors.New("gh pr create: " + strings.Repeat("x", 2000))})
	rec := send(t, h, http.MethodPost, url, shipBody("Fix login", ""), c, nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("ship = %d, want 502: %s", rec.Code, rec.Body)
	}
	var body struct{ Error string }
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(body.Error, "open pull request: gh pr create: xxx") || len(body.Error) > maxShipError+3 {
		t.Errorf("error = %s", fmt.Sprintf("%.80q (len %d)", body.Error, len(body.Error)))
	}
}
