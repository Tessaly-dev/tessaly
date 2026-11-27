// SPDX-License-Identifier: Apache-2.0

// Package auth guards the Tessaly API with a login token.
//
// The daemon keeps one random token in a file only your OS user can read.
// Signing in with that token gives the browser a session cookie. Every API
// route except sign-in needs a valid session.
package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	// CookieName is the name of the session cookie.
	CookieName = "tessaly_session"
	// SessionTTL is how long a session lasts after sign-in.
	SessionTTL = 7 * 24 * time.Hour

	tokenBytes   = 32
	maxLoginBody = 4 << 10
)

// tokenLen is the length of a token encoded with base64.RawURLEncoding.
var tokenLen = base64.RawURLEncoding.EncodedLen(tokenBytes)

// LoadOrCreateToken returns the token stored at path. If the file does not
// exist it writes a new random token there with mode 0600. It refuses a file
// that other users can read, or one that is not a plain file.
func LoadOrCreateToken(path string) (string, error) {
	token, err := readToken(path)
	if !errors.Is(err, fs.ErrNotExist) {
		return token, err
	}

	token = newSecret()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- path is the daemon's own token file.
	if err != nil {
		return "", fmt.Errorf("create token file: %w", err)
	}
	if _, err := f.WriteString(token + "\n"); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("write token file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("write token file: %w", err)
	}
	return token, nil
}

func readToken(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("token file %s is not a regular file", path)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return "", fmt.Errorf("token file %s has mode %04o, other users can read it. Run: chmod 600 %s", path, perm, path)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is the daemon's own token file.
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	token := string(bytes.TrimSpace(data))
	if len(token) != tokenLen {
		return "", fmt.Errorf("token file %s is malformed. Delete it and restart to make a new token", path)
	}
	return token, nil
}

// newSecret returns 32 random bytes, base64url encoded.
func newSecret() string {
	b := make([]byte, tokenBytes)
	_, _ = rand.Read(b) // crypto/rand.Read never fails on supported platforms.
	return base64.RawURLEncoding.EncodeToString(b)
}

// Auth checks the login token and tracks signed-in sessions. Sessions live in
// memory, so restarting the daemon signs everyone out.
type Auth struct {
	tokenHash [sha256.Size]byte
	now       func() time.Time

	mu       sync.Mutex
	sessions map[[sha256.Size]byte]time.Time // session hash -> expiry
}

// New returns an Auth that accepts token.
func New(token string) *Auth {
	return &Auth{
		tokenHash: sha256.Sum256([]byte(token)),
		now:       time.Now,
		sessions:  make(map[[sha256.Size]byte]time.Time),
	}
}

// Login handles POST {"token": "..."}. On a match it sets the session cookie
// and replies 204. It replies 401 on a wrong token.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	// Compare hashes so the comparison takes the same time for any input length.
	got := sha256.Sum256([]byte(body.Token))
	if subtle.ConstantTimeCompare(got[:], a.tokenHash[:]) != 1 {
		http.Error(w, "wrong token", http.StatusUnauthorized)
		return
	}

	session := newSecret()
	a.mu.Lock()
	a.dropExpiredLocked()
	a.sessions[sha256.Sum256([]byte(session))] = a.now().Add(SessionTTL)
	a.mu.Unlock()

	http.SetCookie(w, sessionCookie(r, session, int(SessionTTL/time.Second)))
	w.WriteHeader(http.StatusNoContent)
}

// Logout handles POST. It ends the caller's session and clears the cookie.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		a.mu.Lock()
		delete(a.sessions, sha256.Sum256([]byte(c.Value)))
		a.mu.Unlock()
	}
	http.SetCookie(w, sessionCookie(r, "", -1))
	w.WriteHeader(http.StatusNoContent)
}

// sessionCookie builds the session cookie. A negative maxAge deletes it.
func sessionCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	// #nosec G124 -- Secure is set whenever the request came over TLS. The
	// daemon itself serves plain HTTP on 127.0.0.1, where some browsers drop
	// Secure cookies.
	return &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	}
}

// Require wraps next so it only runs for requests with a valid session.
// Other requests get 401.
func (a *Auth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.signedIn(r) {
			http.Error(w, "sign in required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Auth) signedIn(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	if err != nil || len(c.Value) != tokenLen {
		return false
	}
	key := sha256.Sum256([]byte(c.Value))

	a.mu.Lock()
	defer a.mu.Unlock()
	expiry, ok := a.sessions[key]
	if !ok {
		return false
	}
	if !a.now().Before(expiry) {
		delete(a.sessions, key)
		return false
	}
	return true
}

func (a *Auth) dropExpiredLocked() {
	now := a.now()
	for k, expiry := range a.sessions {
		if !now.Before(expiry) {
			delete(a.sessions, k)
		}
	}
}
