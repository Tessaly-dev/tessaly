// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tessaly-dev/tessaly/internal/forge"
	"github.com/tessaly-dev/tessaly/internal/gitx"
	"github.com/tessaly-dev/tessaly/internal/store"
)

// Remote is the git remote Tessaly pushes workspace branches to.
const Remote = "origin"

// Limits on what a pull request may carry.
const (
	MaxTitle = 256
	MaxBody  = 64 << 10
)

// ErrBadTitle means the title is empty, too long or more than one line.
var ErrBadTitle = fmt.Errorf("the title must be one line of 1 to %d characters", MaxTitle)

// ErrBodyTooLong means the description is over MaxBody.
var ErrBodyTooLong = fmt.Errorf("the description must be at most %d bytes", MaxBody)

// ErrNothingToShip means the branch has no commits and no changes to commit.
var ErrNothingToShip = errors.New("this workspace has no changes to ship")

// ShipError is a failed git or gh step. Its text, with credentials hidden,
// tells you what to fix, such as signing in to gh.
type ShipError struct {
	Step string
	Err  error
}

func (e *ShipError) Error() string { return e.Step + ": " + e.Err.Error() }
func (e *ShipError) Unwrap() error { return e.Err }

// Ship commits any changes in the workspace with title as the message,
// pushes its branch to Remote and opens a pull request with title and body.
// Once a pull request is open, Ship only commits and pushes, and the pull
// request picks up the new commits. It refuses while an agent runs.
func (m *Manager) Ship(ctx context.Context, id int64, title, body string, f forge.Forge) (store.Workspace, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > MaxTitle || strings.ContainsAny(title, "\r\n") {
		return store.Workspace{}, ErrBadTitle
	}
	if len(body) > MaxBody {
		return store.Workspace{}, ErrBodyTooLong
	}

	ws, err := m.beginShip(ctx, id)
	if err != nil {
		return ws, err
	}
	defer m.endShip(id)

	dirty, err := gitx.Dirty(ctx, ws.Path)
	if err != nil {
		return ws, err
	}
	if dirty {
		if err := gitx.CommitAll(ctx, ws.Path, title); err != nil {
			return ws, &ShipError{"commit", err}
		}
	}
	ahead, err := gitx.Ahead(ctx, ws.Path, ws.Base, ws.Branch)
	if err != nil {
		return ws, err
	}
	if ahead == 0 {
		return ws, ErrNothingToShip
	}
	if err := gitx.Push(ctx, ws.Path, Remote, ws.Branch); err != nil {
		return ws, &ShipError{"push", err}
	}
	if ws.PRURL != "" {
		return ws, nil
	}
	url, err := f.OpenPR(ctx, ws.Path, ws.Branch, title, body)
	if err != nil {
		return ws, &ShipError{"open pull request", err}
	}
	if err := m.store.SetWorkspacePR(ctx, ws.ID, url); err != nil {
		return ws, err
	}
	ws.PRURL = url
	return ws, nil
}

// beginShip marks workspace id as shipping, so archive and a second ship
// wait their turn. It refuses a workspace without a worktree or with a
// running agent.
func (m *Manager) beginShip(ctx context.Context, id int64) (store.Workspace, error) {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	ws, err := m.store.Workspace(ctx, id)
	if err != nil {
		return ws, err
	}
	if s := State(ws.State); s != Ready && s != Exited {
		return ws, fmt.Errorf("%w: a workspace can ship only when no agent runs in it", ErrConflict)
	}
	if m.shipping[id] {
		return ws, fmt.Errorf("%w: this workspace is already shipping", ErrConflict)
	}
	m.shipping[id] = true
	return ws, nil
}

func (m *Manager) endShip(id int64) {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	delete(m.shipping, id)
}
