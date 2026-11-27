// SPDX-License-Identifier: Apache-2.0

package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) (*Store, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "tessaly.db")
	s, err := Open(context.Background(), file)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, file
}

func TestOpenCreatesPrivateFileAndMigrates(t *testing.T) {
	s, file := openTest(t)
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("database mode = %04o, want 0600", perm)
	}

	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version < 1 {
		t.Errorf("user_version = %d, want at least 1", version)
	}

	// Opening again must not re-run migrations.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(context.Background(), file)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	_ = again.Close()
}

func TestOpenHandlesOddPaths(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a?b#c", "tessaly.db")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), file)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = s.Close()
	if _, err := os.Stat(file); err != nil {
		t.Errorf("database not created at %s: %v", file, err)
	}
}

func TestMigrationVersion(t *testing.T) {
	tests := []struct {
		name    string
		want    int
		wantErr bool
	}{
		{"migrations/0001_repos.sql", 1, false},
		{"migrations/0012_x.sql", 12, false},
		{"migrations/repos.sql", 0, true},
		{"migrations/abc_repos.sql", 0, true},
		{"migrations/0000_zero.sql", 0, true},
	}
	for _, tt := range tests {
		got, err := migrationVersion(tt.name)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("migrationVersion(%q) = %d, %v; want %d, err %v", tt.name, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestRepos(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()

	b, err := s.AddRepo(ctx, "beta", "/src/beta")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddRepo(ctx, "alpha", "/src/alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddRepo(ctx, "beta", "/src/other"); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate name: err = %v, want ErrExists", err)
	}
	if _, err := s.AddRepo(ctx, "gamma", "/src/beta"); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate path: err = %v, want ErrExists", err)
	}

	repos, err := s.Repos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 || repos[0].Name != "alpha" || repos[1].Name != "beta" {
		t.Fatalf("Repos() = %+v, want alpha then beta", repos)
	}

	got, err := s.Repo(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != b {
		t.Errorf("Repo(%d) = %+v, want %+v", b.ID, got, b)
	}

	names, err := s.RepoNames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !names["alpha"] || !names["beta"] || len(names) != 2 {
		t.Errorf("RepoNames() = %v", names)
	}

	if err := s.RemoveRepo(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveRepo(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second remove: err = %v, want ErrNotFound", err)
	}
	if _, err := s.Repo(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Repo after remove: err = %v, want ErrNotFound", err)
	}
}

func TestReposEmptyIsNotNil(t *testing.T) {
	s, _ := openTest(t)
	repos, err := s.Repos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if repos == nil {
		t.Error("Repos() = nil, want empty slice so the API returns []")
	}
}

func TestSetWorkspacePR(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t)
	repo, err := s.AddRepo(ctx, "app", "/src/app")
	if err != nil {
		t.Fatal(err)
	}
	ws := Workspace{RepoID: repo.ID, Slug: "fix", Branch: "tess/fix", Base: "abc", Path: "/w/fix", State: "ready"}
	if err := s.AddWorkspace(ctx, &ws); err != nil {
		t.Fatal(err)
	}
	url := "https://github.com/acme/app/pull/7"
	if err := s.SetWorkspacePR(ctx, ws.ID, url); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Workspace(ctx, ws.ID); err != nil || got.PRURL != url {
		t.Errorf("PRURL = %q, %v; want %q", got.PRURL, err, url)
	}
	if err := s.SetWorkspacePR(ctx, 999, url); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing workspace: err = %v, want ErrNotFound", err)
	}
}

func TestPasskeys(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()

	if has, err := s.HasPasskeys(ctx); err != nil || has {
		t.Fatalf("HasPasskeys on empty = %v, %v", has, err)
	}
	p, err := s.AddPasskey(ctx, []byte{1, 2}, "Laptop", []byte(`{"id":"AQI"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddPasskey(ctx, []byte{1, 2}, "Again", []byte(`{}`)); !errors.Is(err, ErrExists) {
		t.Errorf("same credential twice = %v, want ErrExists", err)
	}
	if err := s.UsePasskey(ctx, p.ID, []byte(`{"id":"AQI","n":1}`)); err != nil {
		t.Fatal(err)
	}
	keys, err := s.Passkeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Name != "Laptop" || string(keys[0].Data) != `{"id":"AQI","n":1}` || keys[0].LastUsedAt.IsZero() {
		t.Errorf("Passkeys = %+v", keys)
	}
	if has, _ := s.HasPasskeys(ctx); !has {
		t.Error("HasPasskeys = false after adding one")
	}
	if err := s.DeletePasskey(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePasskey(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete = %v, want ErrNotFound", err)
	}
	if err := s.UsePasskey(ctx, p.ID, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("use deleted = %v, want ErrNotFound", err)
	}
}

func TestWebAuthnUserIDIsStable(t *testing.T) {
	s, _ := openTest(t)
	first, err := s.WebAuthnUserID(context.Background())
	if err != nil || len(first) != 32 {
		t.Fatalf("WebAuthnUserID = %x, %v", first, err)
	}
	second, err := s.WebAuthnUserID(context.Background())
	if err != nil || !bytes.Equal(first, second) {
		t.Errorf("second call = %x, %v, want %x", second, err, first)
	}
}
