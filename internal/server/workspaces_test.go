// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"testing"

	"github.com/tessaly-dev/tessaly/internal/store"
)

// addRepoWithCommit adds a repo with one commit through the API.
func addRepoWithCommit(t *testing.T, h http.Handler, c *creds) store.Repo {
	t.Helper()
	dir := newRepo(t, "app")
	cmd := exec.Command("git", "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "first")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	rec := send(t, h, http.MethodPost, "/api/v1/repos", pathBody(dir), c, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add repo = %d: %s", rec.Code, rec.Body)
	}
	var repo store.Repo
	if err := json.NewDecoder(rec.Body).Decode(&repo); err != nil {
		t.Fatal(err)
	}
	return repo
}

func wsBody(repoID int64, name string) string {
	return fmt.Sprintf(`{"repoId":%d,"name":%q}`, repoID, name)
}

func TestWorkspacesAPI(t *testing.T) {
	h := testHandler(t, testUI)
	c := signIn(t, h)
	repo := addRepoWithCommit(t, h, c)

	rec := send(t, h, http.MethodPost, "/api/v1/workspaces", wsBody(repo.ID, "fix-login"), c, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body)
	}
	var ws store.Workspace
	if err := json.NewDecoder(rec.Body).Decode(&ws); err != nil {
		t.Fatal(err)
	}
	if ws.State != "ready" || ws.Branch != "tess/fix-login" || ws.RepoID != repo.ID {
		t.Errorf("ws = %+v", ws)
	}

	if rec := send(t, h, http.MethodPost, "/api/v1/workspaces", wsBody(repo.ID, "fix-login"), c, nil); rec.Code != http.StatusConflict {
		t.Errorf("create twice = %d, want 409", rec.Code)
	}

	rec = send(t, h, http.MethodGet, "/api/v1/workspaces", "", c, nil)
	var list []store.Workspace
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != ws.ID {
		t.Errorf("list = %+v, want the one workspace", list)
	}

	// A repo with workspaces cannot be removed.
	delRepo := "/api/v1/repos/" + strconv.FormatInt(repo.ID, 10)
	if rec := send(t, h, http.MethodDelete, delRepo, "", c, nil); rec.Code != http.StatusConflict {
		t.Errorf("remove repo with workspaces = %d, want 409", rec.Code)
	}

	archive := "/api/v1/workspaces/" + strconv.FormatInt(ws.ID, 10) + "/archive"
	rec = send(t, h, http.MethodPost, archive, "", c, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("archive = %d: %s", rec.Code, rec.Body)
	}
	if rec := send(t, h, http.MethodPost, archive, "", c, nil); rec.Code != http.StatusConflict {
		t.Errorf("archive twice = %d, want 409", rec.Code)
	}
	if rec := send(t, h, http.MethodPost, "/api/v1/workspaces/999/archive", "", c, nil); rec.Code != http.StatusNotFound {
		t.Errorf("archive missing = %d, want 404", rec.Code)
	}
}

func TestWorkspacesAPIRejectsBadInput(t *testing.T) {
	h := testHandler(t, testUI)
	c := signIn(t, h)
	repo := addRepoWithCommit(t, h, c)

	tests := []struct {
		name, body string
		want       int
	}{
		{"bad name", wsBody(repo.ID, "Fix Login"), http.StatusBadRequest},
		{"injection attempt", wsBody(repo.ID, "x; rm -rf ~"), http.StatusBadRequest},
		{"flag-like name", wsBody(repo.ID, "-b"), http.StatusBadRequest},
		{"unknown repo", wsBody(999, "fix"), http.StatusBadRequest},
		{"unknown field", `{"repoId":1,"name":"fix","path":"/etc"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := send(t, h, http.MethodPost, "/api/v1/workspaces", tt.body, c, nil); rec.Code != tt.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
}
