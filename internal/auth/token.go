// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// This file holds when the login token works: until the first passkey, or
// once within ResetTokenTTL after tess auth reset.

// ReplaceToken writes a new random token to path with mode 0600, replacing
// the old file in one rename, and returns it.
func ReplaceToken(path string) (string, error) {
	token := newSecret()
	tmp := path + ".new"
	_ = os.Remove(tmp)                                                   // A leftover from a crash. Never follow it.
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- next to the daemon's own token file.
	if err != nil {
		return "", fmt.Errorf("write token file: %w", err)
	}
	if _, err := f.WriteString(token + "\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("write token file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("write token file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("write token file: %w", err)
	}
	return token, nil
}

// ResetTokenTTL is how long the token from tess auth reset works.
const ResetTokenTTL = 15 * time.Minute

// Reset makes token the only way in, once, until until: it ends every
// session and open passkey prompt, clears the sign-in limit, and returns how
// many sessions it ended. Call it with the token ReplaceToken wrote.
func (a *Auth) Reset(token string, until time.Time) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := len(a.sessions)
	a.resetUntil, a.tokenUsed = until, false
	a.tokenHash = sha256.Sum256([]byte(token))
	a.sessions = make(map[[sha256.Size]byte]session)
	a.ceremonies = make(map[[sha256.Size]byte]ceremony)
	a.failed = 0
	return n
}

// parseTokenState reads the token state row. A row it does not understand
// closes token sign-in, so a damaged row never opens it.
func parseTokenState(state string) (resetUntil time.Time, used bool) {
	if state == "" {
		return time.Time{}, false
	}
	if t, ok := strings.CutPrefix(state, "reset:"); ok {
		if until, err := time.Parse(time.RFC3339, t); err == nil {
			return until, false
		}
	}
	return time.Time{}, true
}

// tokenLoginOpen reports whether the token signs in. With
// --allow-token-login it always does. A token from tess auth reset works
// once, until its expiry. Otherwise the token works until the first
// passkey exists.
func (a *Auth) tokenLoginOpen(hasPasskeys bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tokenOpenLocked(hasPasskeys)
}

func (a *Auth) tokenOpenLocked(hasPasskeys bool) bool {
	switch {
	case a.allowTokenLogin:
		return true
	case a.tokenUsed:
		return false
	case !a.resetUntil.IsZero():
		return a.now().Before(a.resetUntil)
	default:
		return !hasPasskeys
	}
}

// takeToken lets one sign-in use the token. A reset token is then used up,
// in memory and in the store, so a second sign-in with it fails even after
// a restart. It returns false if the token closed meanwhile.
func (a *Auth) takeToken(ctx context.Context, hasPasskeys bool) (bool, error) {
	a.mu.Lock()
	if !a.tokenOpenLocked(hasPasskeys) {
		a.mu.Unlock()
		return false, nil
	}
	singleUse := !a.allowTokenLogin && !a.resetUntil.IsZero()
	if singleUse {
		a.resetUntil, a.tokenUsed = time.Time{}, true
	}
	sites := a.sites
	a.mu.Unlock()
	if singleUse && len(sites) > 0 {
		return true, sites[0].store.SetTokenState(ctx, "used")
	}
	return true, nil
}

// ResetNotice handles GET. It replies {"resetAt": "<time>"} after tess auth
// reset ran, so the UI can warn whoever signs in next, or {"resetAt": null}.
func (a *Auth) ResetNotice(w http.ResponseWriter, r *http.Request) {
	p := a.pk()
	if p == nil {
		writeJSON(w, http.StatusOK, map[string]any{"resetAt": nil})
		return
	}
	at, err := p.store.ResetNotice(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	var out *time.Time
	if !at.IsZero() {
		out = &at
	}
	writeJSON(w, http.StatusOK, map[string]any{"resetAt": out})
}

// DismissResetNotice handles DELETE. It clears the reset notice.
func (a *Auth) DismissResetNotice(w http.ResponseWriter, r *http.Request) {
	p := a.pk()
	if p == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := p.store.DismissResetNotice(r.Context()); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
