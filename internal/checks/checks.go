// SPDX-License-Identifier: Apache-2.0

// Package checks keeps the CI check status of each workspace's pull request
// up to date. It asks the forge every few minutes and on demand, and keeps
// the answers in memory.
package checks

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/tessaly-dev/tessaly/internal/forge"
	"github.com/tessaly-dev/tessaly/internal/store"
)

// DefaultEvery is how often the watcher asks for check status.
const DefaultEvery = time.Minute

// Summary is the check status of one pull request.
type Summary struct {
	// State is "pass" when every check passed or was skipped, "fail" when
	// any failed or was cancelled, "pending" while any runs, and "none"
	// when there are no checks.
	State  string        `json:"state"`
	Checks []forge.Check `json:"checks"`
	// Error says why the last update failed. The previous checks stay.
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Summarize returns the overall state of list.
func Summarize(list []forge.Check) string {
	if len(list) == 0 {
		return "none"
	}
	pending := false
	for _, c := range list {
		switch c.Bucket {
		case "fail", "cancel":
			return "fail"
		case "pending":
			pending = true
		}
	}
	if pending {
		return "pending"
	}
	return "pass"
}

// Watcher polls check status for every workspace with a pull request that
// is not archived.
type Watcher struct {
	store *store.Store
	forge forge.Forge
	every time.Duration
	kick  chan struct{}

	mu   sync.Mutex
	byID map[int64]Summary
}

// New returns a Watcher that asks f every interval.
func New(s *store.Store, f forge.Forge, every time.Duration) *Watcher {
	return &Watcher{store: s, forge: f, every: every, kick: make(chan struct{}, 1), byID: make(map[int64]Summary)}
}

// Get returns the last known status of workspace id's pull request.
func (w *Watcher) Get(id int64) (Summary, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	s, ok := w.byID[id]
	return s, ok
}

// Kick asks for an update soon, such as after a push.
func (w *Watcher) Kick() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// Run updates every interval, and on Kick, until ctx ends.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(w.every)
	defer t.Stop()
	for {
		w.Update(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-w.kick:
		}
	}
}

// Update asks the forge once for every workspace with a pull request, one
// at a time, and forgets workspaces that no longer have one.
func (w *Watcher) Update(ctx context.Context) {
	list, err := w.store.Workspaces(ctx)
	if err != nil {
		slog.Error("checks: list workspaces", "err", err)
		return
	}
	keep := make(map[int64]bool)
	for _, ws := range list {
		if ws.PRURL == "" || ws.State == "archived" || ctx.Err() != nil {
			continue
		}
		keep[ws.ID] = true
		checks, err := w.forge.Checks(ctx, ws.Path, ws.PRURL)
		w.mu.Lock()
		s := w.byID[ws.ID]
		if err != nil {
			s.Error = err.Error()
		} else {
			s = Summary{State: Summarize(checks), Checks: checks}
		}
		s.UpdatedAt = time.Now().UTC()
		w.byID[ws.ID] = s
		w.mu.Unlock()
	}
	w.mu.Lock()
	for id := range w.byID {
		if !keep[id] {
			delete(w.byID, id)
		}
	}
	w.mu.Unlock()
}
