// SPDX-License-Identifier: Apache-2.0

package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/tessaly-dev/tessaly/internal/forge"
	"github.com/tessaly-dev/tessaly/internal/gitx"
	"github.com/tessaly-dev/tessaly/internal/store"
	"github.com/tessaly-dev/tessaly/internal/workspace"
)

type workspaceHandlers struct {
	store   *store.Store
	manager *workspace.Manager
	forge   forge.Forge
}

// workspaceView is a workspace as the list shows it, with live state that
// is not stored.
type workspaceView struct {
	store.Workspace
	// Waiting is true when the agent runs but has been quiet for a while,
	// so it probably waits for input.
	Waiting bool `json:"waiting,omitempty"`
}

func (h workspaceHandlers) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.Workspaces(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	views := make([]workspaceView, len(list))
	for i, ws := range list {
		views[i] = workspaceView{Workspace: ws, Waiting: h.manager.Waiting(ws.ID)}
	}
	writeJSON(w, http.StatusOK, views)
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

// maxDiff caps the diff the API returns. The UI shows that it was cut.
const maxDiff = 1 << 20

// diff returns the workspace's changes since its base commit.
func (h workspaceHandlers) diff(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.find(w, r)
	if !ok {
		return
	}
	switch workspace.State(ws.State) {
	case workspace.Creating, workspace.Failed, workspace.Archived:
		writeError(w, http.StatusConflict, "this workspace has no worktree to compare")
		return
	}
	patch, truncated, err := gitx.Diff(r.Context(), ws.Path, ws.Base, maxDiff)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Patch     string `json:"patch"`
		Truncated bool   `json:"truncated"`
	}{patch, truncated})
}

// maxShipError caps the git or gh message a failed ship returns.
const maxShipError = 1000

// ship commits the workspace's changes, pushes its branch and opens a pull
// request, or pushes new commits to the one already open.
func (h workspaceHandlers) ship(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	ws, ok := h.find(w, r)
	// JSON escapes can make the body several times longer than the text.
	if !ok || !readJSONMax(w, r, &body, 4*workspace.MaxBody) {
		return
	}
	ws, err := h.manager.Ship(r.Context(), ws.ID, body.Title, body.Body, h.forge)
	var shipErr *workspace.ShipError
	switch {
	case errors.Is(err, workspace.ErrBadTitle), errors.Is(err, workspace.ErrBodyTooLong):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, workspace.ErrNothingToShip):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, workspace.ErrConflict):
		writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), workspace.ErrConflict.Error()+": "))
	case errors.As(err, &shipErr):
		// The git or gh message says what to fix, such as signing in to
		// gh. Credentials in it are already hidden.
		writeError(w, http.StatusBadGateway, truncate(shipErr.Error(), maxShipError))
	case err != nil:
		serverError(w, err)
	default:
		writeJSON(w, http.StatusOK, ws)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "..."
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
