// SPDX-License-Identifier: Apache-2.0

package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/tessaly-dev/tessaly/internal/gitx"
	"github.com/tessaly-dev/tessaly/internal/store"
	"github.com/tessaly-dev/tessaly/internal/workspace"
)

type workspaceHandlers struct {
	store   *store.Store
	manager *workspace.Manager
}

func (h workspaceHandlers) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.Workspaces(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h workspaceHandlers) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RepoID int64  `json:"repoId"`
		Name   string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	repo, err := h.store.Repo(r.Context(), body.RepoID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "no such repository")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}

	ws, err := h.manager.Create(r.Context(), repo, body.Name)
	switch {
	case errors.Is(err, workspace.ErrBadSlug):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, workspace.ErrBranchExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrExists):
		writeError(w, http.StatusConflict, "this repository already has a workspace with this name")
	case err != nil:
		serverError(w, err)
	default:
		writeJSON(w, http.StatusCreated, ws)
	}
}

func (h workspaceHandlers) archive(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.find(w, r)
	if !ok {
		return
	}
	repo, err := h.store.Repo(r.Context(), ws.RepoID)
	if err != nil {
		serverError(w, err)
		return
	}
	ws, err = h.manager.Archive(r.Context(), repo, ws)
	switch {
	case errors.Is(err, workspace.ErrConflict):
		writeError(w, http.StatusConflict, "this workspace cannot be archived in its current state")
	case errors.Is(err, gitx.ErrDirtyWorktree):
		writeError(w, http.StatusConflict, err.Error()+". Commit or remove them first")
	case err != nil:
		serverError(w, err)
	default:
		writeJSON(w, http.StatusOK, ws)
	}
}

// find loads the workspace named by the {id} path value, or replies 404.
func (h workspaceHandlers) find(w http.ResponseWriter, r *http.Request) (store.Workspace, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err == nil {
		ws, err := h.store.Workspace(r.Context(), id)
		if err == nil {
			return ws, true
		}
		if !errors.Is(err, store.ErrNotFound) {
			serverError(w, err)
			return store.Workspace{}, false
		}
	}
	writeError(w, http.StatusNotFound, "no such workspace")
	return store.Workspace{}, false
}
