// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/tessaly-dev/tessaly/internal/store"
)

const (
	// PasskeyHost is the only host name passkeys work on. WebAuthn does not
	// accept an IP address such as 127.0.0.1 as a site, and browsers treat
	// http://localhost as secure, so passkeys need http://localhost:<port>.
	PasskeyHost = "localhost"

	// FreshFor is how long a passkey check counts as fresh. Changing your
	// passkeys needs a check this recent, even inside a signed-in session.
	FreshFor = 5 * time.Minute

	// ceremonyTTL is how long a passkey prompt can stay open.
	ceremonyTTL = 2 * time.Minute
	// maxCeremonies caps open passkey prompts, so callers cannot fill memory.
	maxCeremonies = 32
	// maxPasskeyBody caps a WebAuthn reply. Real ones are a few KiB.
	maxPasskeyBody = 64 << 10
	maxPasskeyName = 64
)

// Ceremony kinds. A ceremony started for one kind cannot finish another.
const (
	kindRegister = "register"
	kindLogin    = "login"
	kindVerify   = "verify"
)

// ceremony is one open passkey prompt.
type ceremony struct {
	kind    string
	data    webauthn.SessionData
	session [sha256.Size]byte // The session that started it. Zero for sign-in.
	expiry  time.Time
}

// passkeys holds what passkey sign-in needs.
type passkeys struct {
	store  *store.Store
	rp     *webauthn.WebAuthn
	host   string // "localhost:<port>"
	origin string // "http://localhost:<port>"
}

// EnablePasskeys turns on passkey sign-in, with passkeys kept in db and
// accepted only from http://localhost:<port>. Once a passkey exists, token
// sign-in turns off unless allowTokenLogin is true.
func (a *Auth) EnablePasskeys(db *store.Store, port int, allowTokenLogin bool) error {
	host := net.JoinHostPort(PasskeyHost, strconv.Itoa(port))
	origin := "http://" + host
	rp, err := webauthn.New(&webauthn.Config{
		RPID:          PasskeyHost,
		RPDisplayName: "Tessaly",
		RPOrigins:     []string{origin},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTTL, TimeoutUVD: ceremonyTTL},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTTL, TimeoutUVD: ceremonyTTL},
		},
	})
	if err != nil {
		return fmt.Errorf("passkeys: %w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.passkeys = &passkeys{store: db, rp: rp, host: host, origin: origin}
	a.allowTokenLogin = allowTokenLogin
	return nil
}

// user is the one Tessaly user, as WebAuthn sees it.
type user struct {
	id    []byte
	creds []webauthn.Credential
	ids   []int64 // Store IDs, in the same order as creds.
}

func (u *user) WebAuthnID() []byte                         { return u.id }
func (u *user) WebAuthnName() string                       { return "tessaly" }
func (u *user) WebAuthnDisplayName() string                { return "Tessaly" }
func (u *user) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// loadUser reads the user ID and every passkey from the store.
func (p *passkeys) loadUser(ctx context.Context) (*user, error) {
	id, err := p.store.WebAuthnUserID(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := p.store.Passkeys(ctx)
	if err != nil {
		return nil, err
	}
	u := &user{id: id}
	for _, k := range keys {
		var c webauthn.Credential
		if err := json.Unmarshal(k.Data, &c); err != nil {
			return nil, fmt.Errorf("passkey %d: %w", k.ID, err)
		}
		u.creds = append(u.creds, c)
		u.ids = append(u.ids, k.ID)
	}
	return u, nil
}

// descriptors lists u's passkeys for a WebAuthn prompt.
func (u *user) descriptors() []protocol.CredentialDescriptor {
	out := make([]protocol.CredentialDescriptor, 0, len(u.creds))
	for _, c := range u.creds {
		out = append(out, c.Descriptor())
	}
	return out
}

// LoginOptions handles GET. It tells the sign-in page which ways to sign in
// are open: {"token": bool, "passkey": bool, "passkeyOrigin": "..."}.
// passkeyOrigin is the address passkeys work on, or "" when they are off.
func (a *Auth) LoginOptions(w http.ResponseWriter, r *http.Request) {
	hasKeys, err := a.hasPasskeys(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	origin := ""
	if p := a.pk(); p != nil {
		origin = p.origin
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":         a.tokenLoginOpen(hasKeys),
		"passkey":       hasKeys,
		"passkeyOrigin": origin,
	})
}

func (a *Auth) pk() *passkeys {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.passkeys
}

func (a *Auth) hasPasskeys(ctx context.Context) (bool, error) {
	p := a.pk()
	if p == nil {
		return false, nil
	}
	return p.store.HasPasskeys(ctx)
}

// tokenLoginOpen reports whether the token signs in: always before the first
// passkey, and after it only with --allow-token-login.
func (a *Auth) tokenLoginOpen(hasPasskeys bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !hasPasskeys || a.allowTokenLogin
}

// PasskeyLoginBegin handles POST with no body. It replies
// {"ceremony": "...", "options": {...}} to pass to navigator.credentials.get.
func (a *Auth) PasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	if a.limited(w) {
		return
	}
	p, ok := a.passkeyHost(w, r)
	if !ok {
		return
	}
	u, err := p.loadUser(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	if len(u.creds) == 0 {
		writeError(w, http.StatusConflict, "no passkey is registered yet. Sign in with the token first")
		return
	}
	a.begin(w, kindLogin, [sha256.Size]byte{}, func() (any, *webauthn.SessionData, error) {
		return p.rp.BeginLogin(u, webauthn.WithAllowedCredentials(u.descriptors()),
			webauthn.WithUserVerification(protocol.VerificationRequired))
	})
}

// PasskeyLoginFinish handles POST {"ceremony": "...", "credential": {...}}.
// On a good passkey it starts a session like Login does. A bad reply counts
// as a wrong token for the sign-in limit.
func (a *Auth) PasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	if a.limited(w) {
		return
	}
	p, ok := a.passkeyHost(w, r)
	if !ok {
		return
	}
	body, c, ok := a.finish(w, r, kindLogin, [sha256.Size]byte{})
	if !ok {
		return
	}
	keyID, err := p.assert(r.Context(), c, body.Credential)
	if err != nil {
		a.countFailure()
		slog.Info("passkey sign-in refused", "err", err)
		writeError(w, http.StatusUnauthorized, "that passkey did not sign in")
		return
	}
	a.startSession(w, r, keyID)
}

// assert checks a WebAuthn assertion against c and the stored passkeys, and
// saves the passkey's new counter. It returns the passkey's store ID.
func (p *passkeys) assert(ctx context.Context, c ceremony, raw json.RawMessage) (int64, error) {
	parsed, err := protocol.ParseCredentialRequestResponseBytes(raw)
	if err != nil {
		return 0, err
	}
	u, err := p.loadUser(ctx)
	if err != nil {
		return 0, err
	}
	cred, err := p.rp.ValidateLogin(u, c.data, parsed)
	if err != nil {
		return 0, err
	}
	if cred.Authenticator.CloneWarning {
		return 0, errors.New("the passkey's counter went backwards, so it may be cloned")
	}
	for i, known := range u.creds {
		if !bytes.Equal(known.ID, cred.ID) {
			continue
		}
		data, err := json.Marshal(cred)
		if err != nil {
			return 0, err
		}
		return u.ids[i], p.store.UsePasskey(ctx, u.ids[i], data)
	}
	return 0, errors.New("passkey not found")
}

// PasskeyInfo is a registered passkey as the API shows it.
type PasskeyInfo struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

// ListPasskeys handles GET. It replies with the registered passkeys, and
// whether this session's passkey check is fresh.
func (a *Auth) ListPasskeys(w http.ResponseWriter, r *http.Request) {
	p := a.pk()
	if p == nil {
		writeError(w, http.StatusNotFound, "passkeys are off")
		return
	}
	keys, err := p.store.Passkeys(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	out := make([]PasskeyInfo, 0, len(keys))
	for _, k := range keys {
		info := PasskeyInfo{ID: k.ID, Name: k.Name, CreatedAt: k.CreatedAt}
		if !k.LastUsedAt.IsZero() {
			used := k.LastUsedAt
			info.LastUsedAt = &used
		}
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"passkeys": out,
		"fresh":    a.isFresh(sessionID(r)),
		"origin":   p.origin,
	})
}

// RegisterBegin handles POST with no body from a signed-in session. Adding a
// second passkey needs a fresh passkey check, so a token session cannot add
// its own passkey next to yours.
func (a *Auth) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	p, ok := a.passkeyHost(w, r)
	if !ok {
		return
	}
	u, err := p.loadUser(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	id := sessionID(r)
	if len(u.creds) > 0 && !a.isFresh(id) {
		writeError(w, http.StatusForbidden, "confirm with one of your passkeys first")
		return
	}
	a.begin(w, kindRegister, id, func() (any, *webauthn.SessionData, error) {
		return p.rp.BeginRegistration(u,
			webauthn.WithExclusions(u.descriptors()),
			webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
			webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
				ResidentKey:      protocol.ResidentKeyRequirementPreferred,
				UserVerification: protocol.VerificationRequired,
			}),
			webauthn.WithConveyancePreference(protocol.PreferNoAttestation))
	})
}

// RegisterFinish handles POST {"ceremony", "name", "credential"} and stores
// the new passkey. The session that registers it counts as freshly checked.
func (a *Auth) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	p, ok := a.passkeyHost(w, r)
	if !ok {
		return
	}
	id := sessionID(r)
	body, c, ok := a.finish(w, r, kindRegister, id)
	if !ok {
		return
	}
	name, err := passkeyName(body.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(body.Credential)
	if err != nil {
		writeError(w, http.StatusBadRequest, "that passkey reply is malformed")
		return
	}
	u, err := p.loadUser(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	cred, err := p.rp.CreateCredential(u, c.data, parsed)
	if err != nil {
		slog.Info("passkey registration refused", "err", err)
		writeError(w, http.StatusBadRequest, "that passkey could not be added")
		return
	}
	data, err := json.Marshal(cred)
	if err != nil {
		serverError(w, err)
		return
	}
	key, err := p.store.AddPasskey(r.Context(), cred.ID, name, data)
	if errors.Is(err, store.ErrExists) {
		writeError(w, http.StatusConflict, "that passkey is already registered")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	a.markFresh(id)
	writeJSON(w, http.StatusCreated, PasskeyInfo{ID: key.ID, Name: key.Name, CreatedAt: key.CreatedAt})
}

// passkeyName checks a passkey name and fills in a default.
func passkeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Passkey", nil
	}
	if utf8.RuneCountInString(name) > maxPasskeyName {
		return "", fmt.Errorf("a passkey name can be at most %d characters", maxPasskeyName)
	}
	for _, c := range name {
		if unicode.IsControl(c) {
			return "", errors.New("a passkey name cannot hold control characters")
		}
	}
	return name, nil
}

// DeletePasskey handles DELETE /{id}. It needs a fresh passkey check, and it
// ends every session that signed in with that passkey.
func (a *Auth) DeletePasskey(w http.ResponseWriter, r *http.Request) {
	p := a.pk()
	if p == nil {
		writeError(w, http.StatusNotFound, "passkeys are off")
		return
	}
	keyID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || keyID < 1 {
		writeError(w, http.StatusBadRequest, "bad passkey id")
		return
	}
	if !a.isFresh(sessionID(r)) {
		writeError(w, http.StatusForbidden, "confirm with one of your passkeys first")
		return
	}
	switch err := p.store.DeletePasskey(r.Context(), keyID); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "passkey not found")
		return
	case err != nil:
		serverError(w, err)
		return
	}
	a.endSessionsOf(keyID)
	w.WriteHeader(http.StatusNoContent)
}

// VerifyBegin handles POST with no body. It starts a passkey check for the
// signed-in session. Use it before anything that needs RequireFresh.
func (a *Auth) VerifyBegin(w http.ResponseWriter, r *http.Request) {
	p, ok := a.passkeyHost(w, r)
	if !ok {
		return
	}
	u, err := p.loadUser(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	if len(u.creds) == 0 {
		writeError(w, http.StatusConflict, "add a passkey first")
		return
	}
	a.begin(w, kindVerify, sessionID(r), func() (any, *webauthn.SessionData, error) {
		return p.rp.BeginLogin(u, webauthn.WithAllowedCredentials(u.descriptors()),
			webauthn.WithUserVerification(protocol.VerificationRequired))
	})
}

// VerifyFinish handles POST {"ceremony", "credential"}. A good passkey marks
// the session as freshly checked for FreshFor.
func (a *Auth) VerifyFinish(w http.ResponseWriter, r *http.Request) {
	p, ok := a.passkeyHost(w, r)
	if !ok {
		return
	}
	id := sessionID(r)
	body, c, ok := a.finish(w, r, kindVerify, id)
	if !ok {
		return
	}
	if _, err := p.assert(r.Context(), c, body.Credential); err != nil {
		slog.Info("passkey check refused", "err", err)
		// 403, not 401: the session is still good, only the check failed.
		writeError(w, http.StatusForbidden, "that passkey did not confirm")
		return
	}
	a.markFresh(id)
	w.WriteHeader(http.StatusNoContent)
}

// RequireFresh wraps next so it only runs for a session whose last passkey
// check is at most FreshFor old. Others get 403 and should call VerifyBegin.
// It must sit inside Require.
func (a *Auth) RequireFresh(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.isFresh(sessionID(r)) {
			writeError(w, http.StatusForbidden, "confirm with one of your passkeys first")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// passkeyHost returns the passkey settings when r came to the passkey
// address. Elsewhere it replies 400 with the address to use.
func (a *Auth) passkeyHost(w http.ResponseWriter, r *http.Request) (*passkeys, bool) {
	p := a.pk()
	if p == nil {
		writeError(w, http.StatusNotFound, "passkeys are off")
		return nil, false
	}
	if !strings.EqualFold(r.Host, p.host) {
		writeError(w, http.StatusBadRequest, "passkeys only work at "+p.origin)
		return nil, false
	}
	return p, true
}

// begin runs start, keeps its session data as a new ceremony tied to
// session, and replies {"ceremony": id, "options": ...}.
func (a *Auth) begin(w http.ResponseWriter, kind string, session [sha256.Size]byte,
	start func() (any, *webauthn.SessionData, error),
) {
	options, data, err := start()
	if err != nil {
		serverError(w, err)
		return
	}
	id := newSecret()
	a.mu.Lock()
	now := a.now()
	for k, c := range a.ceremonies {
		if !now.Before(c.expiry) {
			delete(a.ceremonies, k)
		}
	}
	if len(a.ceremonies) >= maxCeremonies {
		a.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "too many passkey prompts are open, try again in a minute")
		return
	}
	a.ceremonies[sha256.Sum256([]byte(id))] = ceremony{
		kind: kind, data: *data, session: session, expiry: now.Add(ceremonyTTL),
	}
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ceremony": id, "options": options})
}

// finishBody is what the UI posts to finish a ceremony.
type finishBody struct {
	Ceremony   string          `json:"ceremony"`
	Name       string          `json:"name"`
	Credential json.RawMessage `json:"credential"`
}

// finish reads the request body and takes its ceremony out of the open
// list, so each ceremony works at most once. The ceremony must be of kind
// and started by session. Otherwise it replies 400 and returns false.
func (a *Auth) finish(w http.ResponseWriter, r *http.Request, kind string, session [sha256.Size]byte,
) (finishBody, ceremony, bool) {
	var body finishBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPasskeyBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || len(body.Ceremony) != tokenLen || len(body.Credential) == 0 {
		writeError(w, http.StatusBadRequest, "bad request body")
		return body, ceremony{}, false
	}
	key := sha256.Sum256([]byte(body.Ceremony))
	a.mu.Lock()
	c, ok := a.ceremonies[key]
	delete(a.ceremonies, key)
	now := a.now()
	a.mu.Unlock()
	if !ok || c.kind != kind || c.session != session || !now.Before(c.expiry) {
		writeError(w, http.StatusBadRequest, "that passkey prompt expired. Try again")
		return body, ceremony{}, false
	}
	return body, c, true
}

// markFresh records a passkey check for the session with id.
func (a *Auth) markFresh(id [sha256.Size]byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s, ok := a.sessions[id]; ok {
		s.verifiedAt = a.now()
		a.sessions[id] = s
	}
}

// isFresh reports whether the session with id had a passkey check in the
// last FreshFor.
func (a *Auth) isFresh(id [sha256.Size]byte) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[id]
	return ok && !s.verifiedAt.IsZero() && a.now().Sub(s.verifiedAt) < FreshFor
}

// endSessionsOf ends every session tied to the passkey with keyID.
func (a *Auth) endSessionsOf(keyID int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, s := range a.sessions {
		if s.passkeyID == keyID {
			delete(a.sessions, k)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError replies {"error": msg}. msg is shown to the user.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func serverError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}
