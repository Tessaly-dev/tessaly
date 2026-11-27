// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/tessaly-dev/tessaly/internal/store"
)

// signIn signs in to h and returns the session.
func signIn(t *testing.T, h http.Handler) *creds {
	t.Helper()
	rec := send(t, h, http.MethodPost, "/api/v1/login", `{"token":"`+testToken+`"}`, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d", rec.Code)
	}
	return credsFrom(t, rec)
}

func newRepo(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func pathBody(p string) string {
	b, _ := json.Marshal(map[string]string{"path": p})
	return string(b)
}

func TestReposAPI(t *testing.T) {
	h := testHandler(t, testUI)
	c := signIn(t, h)
	repo := newRepo(t, "My App")

	rec := send(t, h, http.MethodPost, "/api/v1/repos", pathBody(repo), c, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add = %d: %s", rec.Code, rec.Body)
	}
	var added store.Repo
	if err := json.NewDecoder(rec.Body).Decode(&added); err != nil {
		t.Fatal(err)
	}
	if added.Name != "my-app" || added.Path != repo {
		t.Errorf("added = %+v, want name my-app and path %s", added, repo)
	}

	if rec := send(t, h, http.MethodPost, "/api/v1/repos", pathBody(repo), c, nil); rec.Code != http.StatusConflict {
		t.Errorf("add twice = %d, want 409", rec.Code)
	}

	// A second folder with the same name gets a numbered name.
	rec = send(t, h, http.MethodPost, "/api/v1/repos", pathBody(newRepo(t, "my-app")), c, nil)
	var second store.Repo
	_ = json.NewDecoder(rec.Body).Decode(&second)
	if rec.Code != http.StatusCreated || second.Name != "my-app-2" {
		t.Errorf("second add = %d %+v, want 201 and name my-app-2", rec.Code, second)
	}

	rec = send(t, h, http.MethodGet, "/api/v1/repos", "", c, nil)
	var list []store.Repo
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(list) != 2 {
		t.Fatalf("list = %d %+v, want 2 repos", rec.Code, list)
	}

	del := "/api/v1/repos/" + strconv.FormatInt(added.ID, 10)
	if rec := send(t, h, http.MethodDelete, del, "", c, nil); rec.Code != http.StatusNoContent {
		t.Errorf("remove = %d, want 204", rec.Code)
	}
	if rec := send(t, h, http.MethodDelete, del, "", c, nil); rec.Code != http.StatusNotFound {
		t.Errorf("remove twice = %d, want 404", rec.Code)
	}
	if rec := send(t, h, http.MethodDelete, "/api/v1/repos/abc", "", c, nil); rec.Code != http.StatusNotFound {
		t.Errorf("remove bad id = %d, want 404", rec.Code)
	}
}

func TestReposAPIRejectsBadInput(t *testing.T) {
	h := testHandler(t, testUI)
	c := signIn(t, h)
	repo := newRepo(t, "app")

	tests := []struct {
		name    string
		body    string
		headers map[string]string
		want    int
	}{
		{"not a repo", pathBody(t.TempDir()), nil, http.StatusBadRequest},
		{"subfolder", pathBody(filepath.Join(repo, ".git")), nil, http.StatusBadRequest},
		{"relative", pathBody("app"), nil, http.StatusBadRequest},
		{"unknown field", `{"path":"` + repo + `","cmd":"rm"}`, nil, http.StatusBadRequest},
		{"not json", "path=" + repo, nil, http.StatusBadRequest},
		{"form content type", pathBody(repo), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, http.StatusUnsupportedMediaType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := send(t, h, http.MethodPost, "/api/v1/repos", tt.body, c, tt.headers); rec.Code != tt.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

func TestReposAPINeedsSignIn(t *testing.T) {
	h := testHandler(t, testUI)
	for _, m := range []string{http.MethodGet, http.MethodPost} {
		if rec := send(t, h, m, "/api/v1/repos", pathBody("/"), nil, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without session = %d, want 401", m, rec.Code)
		}
	}
}

func TestRepoSlug(t *testing.T) {
	tests := map[string]string{
		"/src/tessaly": "tessaly",
		"/src/My App":  "my-app",
		"/src/../..":   "repo",
		"/src/__x__":   "x",
		"/src/日本":      "repo",
		"/src/a-very-long-folder-name-that-goes-past-forty-chars": "a-very-long-folder-name-that-goes-past-f",
	}
	for in, want := range tests {
		if got := repoSlug(in); got != want {
			t.Errorf("repoSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniqueName(t *testing.T) {
	taken := map[string]bool{"app": true, "app-2": true}
	if got := uniqueName("app", taken); got != "app-3" {
		t.Errorf("uniqueName = %q, want app-3", got)
	}
	if got := uniqueName("web", taken); got != "web" {
		t.Errorf("uniqueName = %q, want web", got)
	}
}
