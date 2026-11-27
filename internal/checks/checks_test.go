// SPDX-License-Identifier: Apache-2.0

package checks

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tessaly-dev/tessaly/internal/forge"
	"github.com/tessaly-dev/tessaly/internal/store"
)

type fakeForge struct {
	mu     sync.Mutex
	checks []forge.Check
	err    error
	asked  []string
}

func (f *fakeForge) OpenPR(context.Context, string, string, string, string) (string, error) {
	return "", errors.New("not used")
}

func (f *fakeForge) Checks(_ context.Context, _, prURL string) ([]forge.Check, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, prURL)
	return f.checks, f.err
}

func TestSummarize(t *testing.T) {
	c := func(buckets ...string) []forge.Check {
		var list []forge.Check
		for _, b := range buckets {
			list = append(list, forge.Check{Name: b, Bucket: b})
		}
		return list
	}
	tests := []struct {
		list []forge.Check
		want string
	}{
		{nil, "none"},
		{c("pass", "skipping"), "pass"},
		{c("pass", "pending"), "pending"},
		{c("pending", "fail"), "fail"},
		{c("pass", "cancel"), "fail"},
	}
	for _, tt := range tests {
		if got := Summarize(tt.list); got != tt.want {
			t.Errorf("Summarize(%v) = %q, want %q", tt.list, got, tt.want)
		}
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "tessaly.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	repo, err := s.AddRepo(ctx, "app", "/src/app")
	if err != nil {
		t.Fatal(err)
	}
	add := func(slug, state, pr string) store.Workspace {
		ws := store.Workspace{RepoID: repo.ID, Slug: slug, Branch: "tess/" + slug, Base: "b", Path: "/w/" + slug, State: state}
		if err := s.AddWorkspace(ctx, &ws); err != nil {
			t.Fatal(err)
		}
		if pr != "" {
			if err := s.SetWorkspacePR(ctx, ws.ID, pr); err != nil {
				t.Fatal(err)
			}
		}
		return ws
	}
	withPR := add("fix", "exited", "https://github.com/a/b/pull/1")
	noPR := add("other", "exited", "")
	archived := add("old", "archived", "https://github.com/a/b/pull/2")

	f := &fakeForge{checks: []forge.Check{{Name: "CI", Bucket: "fail"}}}
	w := New(s, f, time.Hour)
	w.Update(ctx)

	if got, ok := w.Get(withPR.ID); !ok || got.State != "fail" || len(got.Checks) != 1 {
		t.Errorf("Get(with PR) = %+v, %v", got, ok)
	}
	for _, id := range []int64{noPR.ID, archived.ID} {
		if _, ok := w.Get(id); ok {
			t.Errorf("Get(%d) found a status, want none", id)
		}
	}
	if len(f.asked) != 1 {
		t.Errorf("asked %v, want only the one open pull request", f.asked)
	}

	// A failed update keeps the last checks and records the error.
	f.err = errors.New("gh pr checks: offline")
	w.Update(ctx)
	if got, _ := w.Get(withPR.ID); got.State != "fail" || got.Error != "gh pr checks: offline" {
		t.Errorf("after error = %+v", got)
	}

	// An archived workspace is forgotten.
	if err := s.SetWorkspaceState(ctx, withPR.ID, "archived", ""); err != nil {
		t.Fatal(err)
	}
	w.Update(ctx)
	if _, ok := w.Get(withPR.ID); ok {
		t.Error("archived workspace still has a status")
	}
}

func TestRunStopsAndKicks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "tessaly.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	w := New(s, &fakeForge{}, time.Hour)
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	w.Kick()
	w.Kick() // A second kick while one waits must not block.
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}
