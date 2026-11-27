// SPDX-License-Identifier: Apache-2.0

// Package auth guards the Tessaly API with passkeys and a login token.
//
// The daemon keeps one random token in a file only your OS user can read.
// You sign in with that token until you register a passkey. After that only
// a passkey signs in, unless tess serve runs with --allow-token-login.
// Either way, signing in starts a session made of two secrets: an
// HttpOnly cookie, and a session key the UI keeps in localStorage and sends
// in the X-Tessaly-Key header. Every API route except sign-in needs both.
//
// Browsers send cookies for 127.0.0.1 to every port on it, so any local server
// you open in the browser gets the cookie. localStorage is scoped to one
// origin, port included, so that server never sees the key, and the cookie
// alone is worthless. The cookie stays HttpOnly, so a script cannot take the
// whole session with it either.
package auth

import (
	"bytes"
	"context"
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
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// CookieName is the name of the session cookie.
	CookieName = "tessaly_session"
	// KeyHeader carries the session key on API requests.
	KeyHeader = "X-Tessaly-Key"
	// KeyProtocol prefixes the session key in Sec-WebSocket-Protocol, since
	// browsers cannot set other headers on a WebSocket.
	KeyProtocol = "tessaly.key."
	// Protocol is the WebSocket subprotocol the server picks. It never
	// echoes the key back.
	Protocol = "tessaly"
	// SessionTTL is how long a session lasts after sign-in.
	SessionTTL = 7 * 24 * time.Hour

	// MaxFailedLogins is how many wrong tokens Login accepts per
	// FailedLoginWindow. Past that it replies 429 to every sign-in, right
	// token or not, until the window ends.
	MaxFailedLogins = 10
	// FailedLoginWindow is the window for MaxFailedLogins.
	FailedLoginWindow = time.Minute

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
	sessions map[[sha256.Size]byte]session // cookie hash -> session
	// failed counts wrong tokens and passkeys since failedSince.
	failed      int
	failedSince time.Time

	// passkeys is nil until EnablePasskeys.
	passkeys        *passkeys
	allowTokenLogin bool
	ceremonies      map[[sha256.Size]byte]ceremony // ceremony ID hash -> ceremony
}

// session is one sign-in. Only hashes of its secrets are kept.
type session struct {
	keyHash [sha256.Size]byte
	expiry  time.Time
	// passkeyID is the passkey that signed in, or 0 for the token.
	passkeyID int64
	// verifiedAt is the time of the last passkey check, for RequireFresh.
	verifiedAt time.Time
}

// New returns an Auth that accepts token. Call EnablePasskeys to add
// passkey sign-in.
func New(token string) *Auth {
	return &Auth{
		tokenHash:  sha256.Sum256([]byte(token)),
		now:        time.Now,
		sessions:   make(map[[sha256.Size]byte]session),
		ceremonies: make(map[[sha256.Size]byte]ceremony),
	}
}

// Login handles POST {"token": "..."}. On a match it sets the session cookie
// and replies 200 with {"key": "..."}, the session key. It replies 401 on a
// wrong token, and 403 when token sign-in is off because a passkey exists.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	if a.limited(w) {
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	// Refuse before looking at the token, so a closed sign-in never tells
	// anyone whether a token is right.
	hasKeys, err := a.hasPasskeys(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	if !a.tokenLoginOpen(hasKeys) {
		http.Error(w, "token sign-in is off because a passkey is registered. "+
			"Sign in with your passkey, or run tess serve --allow-token-login", http.StatusForbidden)
		return
	}
	// Compare hashes so the comparison takes the same time for any input length.
	got := sha256.Sum256([]byte(body.Token))
	a.mu.Lock()
	want := a.tokenHash // Reset can change it.
	a.mu.Unlock()
	if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
		a.countFailure()
		http.Error(w, "wrong token", http.StatusUnauthorized)
		return
	}
	a.startSession(w, r, 0)
}

// startSession makes a new session, sets its cookie and replies with its
// key. passkeyID is the passkey that signed in, or 0 for the token. A
// passkey sign-in counts as a fresh passkey check.
func (a *Auth) startSession(w http.ResponseWriter, r *http.Request, passkeyID int64) {
	cookie, key := newSecret(), newSecret()
	a.mu.Lock()
	a.dropExpiredLocked()
	s := session{
		keyHash:   sha256.Sum256([]byte(key)),
		expiry:    a.now().Add(SessionTTL),
		passkeyID: passkeyID,
	}
	if passkeyID != 0 {
		s.verifiedAt = a.now()
	}
	a.sessions[sha256.Sum256([]byte(cookie))] = s
	a.mu.Unlock()

	http.SetCookie(w, sessionCookie(r, cookie, int(SessionTTL/time.Second)))
	writeJSON(w, http.StatusOK, struct {
		Key string `json:"key"`
	}{key})
}

// limited replies 429 and returns true while sign-in is closed after too
// many failures.
func (a *Auth) limited(w http.ResponseWriter) bool {
	wait := a.loginWait()
	if wait <= 0 {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(wait.Round(time.Second)/time.Second)))
	http.Error(w, "too many failed sign-ins, try again later", http.StatusTooManyRequests)
	return true
}

func (a *Auth) countFailure() {
	a.mu.Lock()
	a.failed++
	a.mu.Unlock()
}

// loginWait returns how long sign-in stays closed after too many wrong
// tokens, or 0 when it is open.
func (a *Auth) loginWait() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	if now.Sub(a.failedSince) >= FailedLoginWindow {
		a.failed, a.failedSince = 0, now
	}
	if a.failed < MaxFailedLogins {
		return 0
	}
	return FailedLoginWindow - now.Sub(a.failedSince)
}

// Logout handles POST. It ends the caller's session and clears the cookie.
// The session ends only when the request carries its key too, so a server
// that got the cookie alone cannot sign you out.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if id, ok := a.lookup(r); ok {
		a.mu.Lock()
		delete(a.sessions, id)
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
		id, ok := a.lookup(r)
		if !ok {
			http.Error(w, "sign in required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, id)))
	})
}

type sessionKey struct{}

// sessionID returns the id of the session Require let through, or the zero
// id outside Require.
func sessionID(r *http.Request) [sha256.Size]byte {
	id, _ := r.Context().Value(sessionKey{}).([sha256.Size]byte)
	return id
}

// lookup returns the id of the live session whose cookie and key r carries.
func (a *Auth) lookup(r *http.Request) ([sha256.Size]byte, bool) {
	var id [sha256.Size]byte
	c, err := r.Cookie(CookieName)
	if err != nil || len(c.Value) != tokenLen {
		return id, false
	}
	key := requestKey(r)
	if len(key) != tokenLen {
		return id, false
	}
	id = sha256.Sum256([]byte(c.Value))
	keyHash := sha256.Sum256([]byte(key))

	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[id]
	if !ok {
		return id, false
	}
	if !a.now().Before(s.expiry) {
		delete(a.sessions, id)
		return id, false
	}
	return id, subtle.ConstantTimeCompare(keyHash[:], s.keyHash[:]) == 1
}

// requestKey returns the session key from the X-Tessaly-Key header or, for a
// WebSocket, from a "tessaly.key.<key>" subprotocol. It returns "" if neither
// is there.
func requestKey(r *http.Request) string {
	if k := r.Header.Get(KeyHeader); k != "" {
		return k
	}
	for _, v := range r.Header.Values("Sec-WebSocket-Protocol") {
		for p := range strings.SplitSeq(v, ",") {
			if k, ok := strings.CutPrefix(strings.TrimSpace(p), KeyProtocol); ok {
				return k
			}
		}
	}
	return ""
}

func (a *Auth) dropExpiredLocked() {
	now := a.now()
	for k, s := range a.sessions {
		if !now.Before(s.expiry) {
			delete(a.sessions, k)
		}
	}
}

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

// Reset makes token the only way in: it ends every session and open
// passkey prompt, clears the sign-in limit, and returns how many sessions
// it ended. Call it with the token ReplaceToken wrote.
func (a *Auth) Reset(token string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := len(a.sessions)
	a.tokenHash = sha256.Sum256([]byte(token))
	a.sessions = make(map[[sha256.Size]byte]session)
	a.ceremonies = make(map[[sha256.Size]byte]ceremony)
	a.failed = 0
	return n
}
