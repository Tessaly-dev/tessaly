// SPDX-License-Identifier: Apache-2.0

package server

import (
	"errors"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/tessaly-dev/tessaly/internal/gitx"
	"github.com/tessaly-dev/tessaly/internal/store"
)

// maxJSONBody caps request bodies on the API.
const maxJSONBody = 64 << 10

type repoHandlers struct {
	store *store.Store
	// addMu makes picking a free name and storing it one step.
	addMu *sync.Mutex
}

func (h repoHandlers) list(w http.ResponseWriter, r *http.Request) {
	repos, err := h.store.Repos(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, repos)
}

func (h repoHandlers) add(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	root, err := gitx.RepoRoot(r.Context(), body.Path)
	if err != nil {
		// The error names only the path the caller sent, so it is safe to show.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.addMu.Lock()
	defer h.addMu.Unlock()
	names, err := h.store.RepoNames(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	repo, err := h.store.AddRepo(r.Context(), uniqueName(repoSlug(root), names), root)
	if errors.Is(err, store.ErrExists) {
		writeError(w, http.StatusConflict, "this repository is already added")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, repo)
}

func (h repoHandlers) remove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "no such repository")
		return
	}
	err = h.store.RemoveRepo(r.Context(), id)
	if errors.Is(err, store.ErrInUse) {
		writeError(w, http.StatusConflict, "this repository still has workspaces")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such repository")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// repoSlug turns the folder name into a short name made of [a-z0-9-], used in
// URLs and folder names. It never returns an empty string.
func repoSlug(root string) string {
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(filepath.Base(root)), "-"), "-")
	if len(slug) > 40 {
		slug = strings.TrimRight(slug[:40], "-")
	}
	if slug == "" {
		return "repo"
	}
	return slug
}

// uniqueName returns base, or base-2, base-3 and so on, whichever is free.
func uniqueName(base string, taken map[string]bool) string {
	name := base
	for i := 2; taken[name]; i++ {
		name = base + "-" + strconv.Itoa(i)
	}
	return name
}
