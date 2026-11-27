// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Workspace is one task: one branch and one worktree in one repo.
type Workspace struct {
	ID        int64     `json:"id"`
	RepoID    int64     `json:"repoId"`
	Slug      string    `json:"slug"`
	Branch    string    `json:"branch"`
	Base      string    `json:"base"`
	Path      string    `json:"path"`
	State     string    `json:"state"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	// Agent names the agent that last ran here, if any.
	Agent string `json:"agent,omitempty"`
	// ExitCode is how the last agent run ended. It is nil while one runs or
	// before the first run.
	ExitCode *int `json:"exitCode,omitempty"`
	// Ended says how the last agent run ended: "exited", "stopped" or
	// "interrupted". It is empty while one runs or before the first run.
	Ended string `json:"ended,omitempty"`
}

const workspaceCols = "id, repo_id, slug, branch, base, path, state, error, created_at, agent, exit_code, ended"

// AddWorkspace stores w and sets its ID and CreatedAt. It returns ErrExists
// if the repo already has a workspace with that slug, or the path is taken.
func (s *Store) AddWorkspace(ctx context.Context, w *Workspace) error {
	w.CreatedAt = time.Now().UTC().Truncate(time.Second)
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO workspaces (repo_id, slug, branch, base, path, state, error, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		w.RepoID, w.Slug, w.Branch, w.Base, w.Path, w.State, w.Error, w.CreatedAt.Format(time.RFC3339))
	if err != nil {
		if isUniqueViolation(err) {
			return ErrExists
		}
		return fmt.Errorf("add workspace: %w", err)
	}
	w.ID, err = res.LastInsertId()
	return err
}

// SetWorkspaceState stores a new state and error text for workspace id.
func (s *Store) SetWorkspaceState(ctx context.Context, id int64, state, errText string) error {
	res, err := s.db.ExecContext(ctx, "UPDATE workspaces SET state = ?, error = ? WHERE id = ?", state, errText, id)
	if err != nil {
		return fmt.Errorf("set workspace state: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetWorkspaceRun stores the agent, exit code and end of the latest run. A
// nil exitCode and an empty ended mean the run is still going.
func (s *Store) SetWorkspaceRun(ctx context.Context, id int64, agent string, exitCode *int, ended string) error {
	res, err := s.db.ExecContext(ctx, "UPDATE workspaces SET agent = ?, exit_code = ?, ended = ? WHERE id = ?", agent, exitCode, ended, id)
	if err != nil {
		return fmt.Errorf("set workspace run: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Workspace returns the workspace with id, or ErrNotFound.
func (s *Store) Workspace(ctx context.Context, id int64) (Workspace, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+workspaceCols+" FROM workspaces WHERE id = ?", id)
	w, err := scanWorkspace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	return w, err
}

// Workspaces returns every workspace, newest first.
func (s *Store) Workspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+workspaceCols+" FROM workspaces ORDER BY id DESC")
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	list := []Workspace{}
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, w)
	}
	return list, rows.Err()
}

func scanWorkspace(sc scanner) (Workspace, error) {
	var w Workspace
	var created string
	var exit sql.NullInt64
	if err := sc.Scan(&w.ID, &w.RepoID, &w.Slug, &w.Branch, &w.Base, &w.Path, &w.State, &w.Error, &created, &w.Agent, &exit, &w.Ended); err != nil {
		return Workspace{}, err
	}
	if exit.Valid {
		code := int(exit.Int64)
		w.ExitCode = &code
	}
	t, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return Workspace{}, fmt.Errorf("workspace %d: bad created_at: %w", w.ID, err)
	}
	w.CreatedAt = t
	return w, nil
}
