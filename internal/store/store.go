// SPDX-License-Identifier: Apache-2.0

// Package store keeps Tessaly's state in one SQLite file, tessaly.db.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // Pure Go SQLite driver, registered as "sqlite".
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrNotFound means the row does not exist.
var ErrNotFound = errors.New("not found")

// ErrExists means a row with the same unique value already exists.
var ErrExists = errors.New("already exists")

// Store wraps the database.
type Store struct {
	db *sql.DB
}

// Open opens or creates the database at file with mode 0600 and brings its
// schema up to date.
func Open(ctx context.Context, file string) (*Store, error) {
	// Create the file first so it gets mode 0600 rather than the umask default.
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G304 -- file is the daemon's own database.
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Build a file: URI so a path with "?" or "#" in it cannot change the options.
	dsn := url.URL{
		Scheme:   "file",
		Path:     file,
		RawQuery: "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
	}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// One connection keeps writes ordered and avoids "database is locked".
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// migrate runs every migration file newer than the stored user_version, in
// order, each in its own transaction.
func (s *Store) migrate(ctx context.Context) error {
	var current int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, name := range files {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		if version <= current {
			continue
		}
		body, err := migrations.ReadFile(name)
		if err != nil {
			return err
		}
		if err := s.apply(ctx, version, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) apply(ctx context.Context, version int, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	// PRAGMA does not take bound parameters. version is an int parsed from an
	// embedded file name, so formatting it in is safe.
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(version)); err != nil {
		return err
	}
	return tx.Commit()
}

// migrationVersion parses the number in "migrations/0001_name.sql".
func migrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(path.Base(name), "_")
	if !ok {
		return 0, fmt.Errorf("migration %s: name must start with a number and _", name)
	}
	v, err := strconv.Atoi(prefix)
	if err != nil || v < 1 {
		return 0, fmt.Errorf("migration %s: bad version %q", name, prefix)
	}
	return v, nil
}
