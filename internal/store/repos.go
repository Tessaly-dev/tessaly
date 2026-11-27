// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Repo is a git repository the user added to Tessaly.
type Repo struct {
	ID      int64     `json:"id"`
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	AddedAt time.Time `json:"addedAt"`
}

// AddRepo stores a repo. It returns ErrExists if the name or path is taken.
func (s *Store) AddRepo(ctx context.Context, name, path string) (Repo, error) {
	added := time.Now().UTC().Truncate(time.Second)
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO repos (name, path, added_at) VALUES (?, ?, ?)",
		name, path, added.Format(time.RFC3339))
	if err != nil {
		if isUniqueViolation(err) {
			return Repo{}, ErrExists
		}
		return Repo{}, fmt.Errorf("add repo: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Repo{}, fmt.Errorf("add repo: %w", err)
	}
	return Repo{ID: id, Name: name, Path: path, AddedAt: added}, nil
}

// Repos returns every repo, sorted by name.
func (s *Store) Repos(ctx context.Context) ([]Repo, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, path, added_at FROM repos ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}
	defer func() { _ = rows.Close() }()

	repos := []Repo{}
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// Repo returns the repo with id, or ErrNotFound.
func (s *Store) Repo(ctx context.Context, id int64) (Repo, error) {
	row := s.db.QueryRowContext(ctx, "SELECT id, name, path, added_at FROM repos WHERE id = ?", id)
	r, err := scanRepo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Repo{}, ErrNotFound
	}
	return r, err
}

// RepoNames returns the set of names in use.
func (s *Store) RepoNames(ctx context.Context) (map[string]bool, error) {
	repos, err := s.Repos(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool, len(repos))
	for _, r := range repos {
		names[r.Name] = true
	}
	return names, nil
}

// ErrInUse means other rows still point at the row.
var ErrInUse = errors.New("still in use")

// RemoveRepo deletes the repo record. It never touches files on disk. It
// returns ErrInUse while the repo has workspaces.
func (s *Store) RemoveRepo(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM repos WHERE id = ?", id)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			return ErrInUse
		}
		return fmt.Errorf("remove repo: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("remove repo: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface{ Scan(dest ...any) error }

func scanRepo(sc scanner) (Repo, error) {
	var r Repo
	var added string
	if err := sc.Scan(&r.ID, &r.Name, &r.Path, &added); err != nil {
		return Repo{}, err
	}
	t, err := time.Parse(time.RFC3339, added)
	if err != nil {
		return Repo{}, fmt.Errorf("repo %d: bad added_at: %w", r.ID, err)
	}
	r.AddedAt = t
	return r, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
