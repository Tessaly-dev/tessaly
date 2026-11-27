// SPDX-License-Identifier: Apache-2.0

// Package workspace creates and archives workspaces. A workspace is one task
// in one repo: a new branch named tess/<slug>, checked out in its own git
// worktree under ~/.tessaly/worktrees/<repo>/<slug>.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/tessaly-dev/tessaly/internal/gitx"
	"github.com/tessaly-dev/tessaly/internal/store"
)

// BranchPrefix starts every branch Tessaly makes.
const BranchPrefix = "tess/"

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)

// ErrBadSlug means the slug is not 1 to 48 of [a-z0-9-], starting with a letter or digit.
var ErrBadSlug = errors.New("name must be 1 to 48 lower-case letters, digits or dashes, starting with a letter or digit")

// ErrBranchExists means the branch for the slug already exists in the repo.
// Tessaly never reuses a branch.
var ErrBranchExists = errors.New("a branch with this name already exists")

// ErrConflict means the request does not fit the workspace's current state.
var ErrConflict = errors.New("conflict")

// Manager creates and archives workspaces.
type Manager struct {
	store *store.Store
	// home is the data folder, ~/.tessaly.
	home string
	// root is the folder that holds every worktree, home/worktrees.
	root string

	// runMu guards runs and orders state changes of archive, start and
	// agent exit, so they cannot race.
	runMu sync.Mutex
	runs  map[int64]*run
}

// NewManager returns a Manager that keeps worktrees and transcripts in the
// data folder home.
func NewManager(s *store.Store, home string) *Manager {
	return &Manager{
		store: s,
		home:  home,
		root:  filepath.Join(home, "worktrees"),
		runs:  make(map[int64]*run),
	}
}

// ValidSlug reports whether slug can name a workspace and its branch.
func ValidSlug(slug string) bool { return slugRE.MatchString(slug) }

// Create makes a branch and a worktree for a new workspace in repo, based on
// the repo's current HEAD.
func (m *Manager) Create(ctx context.Context, repo store.Repo, slug string) (store.Workspace, error) {
	if !ValidSlug(slug) {
		return store.Workspace{}, ErrBadSlug
	}
	branch := BranchPrefix + slug
	exists, err := gitx.BranchExists(ctx, repo.Path, branch)
	if err != nil {
		return store.Workspace{}, err
	}
	if exists {
		return store.Workspace{}, ErrBranchExists
	}
	base, err := gitx.Head(ctx, repo.Path)
	if err != nil {
		return store.Workspace{}, fmt.Errorf("the repo has no commit to branch from: %w", err)
	}

	path, err := m.worktreePath(repo.Name, slug)
	if err != nil {
		return store.Workspace{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return store.Workspace{}, fmt.Errorf("create worktree folder: %w", err)
	}
	// Store the real path, as git reports it. On macOS, for example, the
	// temp folder sits behind a symlink.
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return store.Workspace{}, fmt.Errorf("resolve worktree folder: %w", err)
	}
	path = filepath.Join(parent, slug)

	// Store the row first: the unique (repo, slug) key stops two requests
	// from making the same workspace at once.
	ws := store.Workspace{RepoID: repo.ID, Slug: slug, Branch: branch, Base: base, Path: path, State: string(Creating)}
	if err := m.store.AddWorkspace(ctx, &ws); err != nil {
		return store.Workspace{}, err
	}

	if err := gitx.AddWorktree(ctx, repo.Path, path, branch, base); err != nil {
		return m.fail(ctx, ws, err)
	}
	return m.move(ctx, ws, Ready, "")
}

// Archive removes the workspace's worktree and keeps its branch. It refuses a
// worktree with uncommitted or untracked files.
func (m *Manager) Archive(ctx context.Context, repo store.Repo, ws store.Workspace) (store.Workspace, error) {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	// Re-read under the lock, in case another request changed it.
	ws, err := m.store.Workspace(ctx, ws.ID)
	if err != nil {
		return ws, err
	}
	if err := checkMove(State(ws.State), Archived); err != nil {
		return ws, fmt.Errorf("%w: %w", ErrConflict, err)
	}
	var ok bool
	// Only remove a path git lists as this repo's worktree. A failed create
	// may have left nothing, or something that is not ours.
	ok, err = gitx.HasWorktree(ctx, repo.Path, ws.Path)
	if err != nil {
		return ws, err
	}
	if ok {
		if err := gitx.RemoveWorktree(ctx, repo.Path, ws.Path); err != nil {
			return ws, err
		}
	}
	return m.move(ctx, ws, Archived, "")
}

// Recover fixes workspaces that tess left behind when it stopped: Creating
// becomes Failed and Running becomes Exited. Call it once at startup, before
// serving requests.
func (m *Manager) Recover(ctx context.Context) error {
	list, err := m.store.Workspaces(ctx)
	if err != nil {
		return err
	}
	for _, ws := range list {
		var err error
		switch State(ws.State) {
		case Creating:
			_, err = m.move(ctx, ws, Failed, "tess stopped while creating this workspace")
		case Running:
			_, err = m.move(ctx, ws, Exited, "tess stopped while the agent was running")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) move(ctx context.Context, ws store.Workspace, to State, errText string) (store.Workspace, error) {
	if err := checkMove(State(ws.State), to); err != nil {
		return ws, err
	}
	if err := m.store.SetWorkspaceState(ctx, ws.ID, string(to), errText); err != nil {
		return ws, err
	}
	ws.State, ws.Error = string(to), errText
	return ws, nil
}

// fail marks ws as failed with cause and returns cause.
func (m *Manager) fail(ctx context.Context, ws store.Workspace, cause error) (store.Workspace, error) {
	ws, err := m.move(ctx, ws, Failed, cause.Error())
	if err != nil {
		return ws, errors.Join(cause, err)
	}
	return ws, cause
}

// worktreePath returns root/<repo>/<slug> and checks that it stays inside root.
func (m *Manager) worktreePath(repoName, slug string) (string, error) {
	p := filepath.Join(m.root, repoName, slug)
	rel, err := filepath.Rel(m.root, p)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worktree path %q leaves %q", p, m.root)
	}
	return p, nil
}
