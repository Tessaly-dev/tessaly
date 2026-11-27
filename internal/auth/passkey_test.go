// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"

	"github.com/tessaly-dev/tessaly/internal/store"
)

const (
	testPort   = 7777
	testHost   = "localhost:7777"
	testOrigin = "http://localhost:7777"
)

var b64 = base64.RawURLEncoding

// authenticator is a software passkey. It signs like a real one, so tests
// run the whole WebAuthn check without a browser.
type authenticator struct {
	key     *ecdsa.PrivateKey
	id      []byte
	counter uint32
	origin  string // The origin it reports. Defaults to testOrigin.
	noUV    bool   // Leave out the user verification flag.
}

func newAuthenticator(t *testing.T) *authenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &authenticator{key: key, id: id, origin: testOrigin}
}

func (a *authenticator) clientData(t *testing.T, typ, challenge string) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]string{"type": typ, "challenge": challenge, "origin": a.origin})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// authData builds authenticator data for rpID "localhost". attested holds
// the attested credential data on registration.
func (a *authenticator) authData(attested []byte) []byte {
	rp := sha256.Sum256([]byte("localhost"))
	flags := byte(0x01) // User present.
	if !a.noUV {
		flags |= 0x04
	}
	if attested != nil {
		flags |= 0x40
	}
	out := append([]byte{}, rp[:]...)
	out = append(out, flags)
	out = binary.BigEndian.AppendUint32(out, a.counter)
	return append(out, attested...)
}

// create answers navigator.credentials.create options with a credential.
func (a *authenticator) create(t *testing.T, options json.RawMessage) json.RawMessage {
	t.Helper()
	var opts struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &opts); err != nil {
		t.Fatal(err)
	}
	cose := webauthncose.EC2PublicKeyData{
		PublicKeyData: webauthncose.PublicKeyData{KeyType: 2, Algorithm: -7},
		Curve:         1,
		XCoord:        a.key.X.FillBytes(make([]byte, 32)),
		YCoord:        a.key.Y.FillBytes(make([]byte, 32)),
	}
	coseKey, err := webauthncbor.Marshal(cose)
	if err != nil {
		t.Fatal(err)
	}
	attested := make([]byte, 16)                                          // Zero AAGUID.
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(a.id))) // #nosec G115 -- a.id is 16 bytes.
	attested = append(attested, a.id...)
	attested = append(attested, coseKey...)
	object, err := webauthncbor.Marshal(map[string]any{
		"fmt": "none", "attStmt": map[string]any{}, "authData": a.authData(attested),
	})
	if err != nil {
		t.Fatal(err)
	}
	return mustJSON(t, map[string]any{
		"id": b64.EncodeToString(a.id), "rawId": b64.EncodeToString(a.id), "type": "public-key",
		"response": map[string]string{
			"clientDataJSON":    b64.EncodeToString(a.clientData(t, "webauthn.create", opts.PublicKey.Challenge)),
			"attestationObject": b64.EncodeToString(object),
		},
	})
}

// get answers navigator.credentials.get options with a signed assertion.
func (a *authenticator) get(t *testing.T, options json.RawMessage) json.RawMessage {
	t.Helper()
	var opts struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &opts); err != nil {
		t.Fatal(err)
	}
	a.counter++
	clientData := a.clientData(t, "webauthn.get", opts.PublicKey.Challenge)
	authData := a.authData(nil)
	hash := sha256.Sum256(clientData)
	digest := sha256.Sum256(append(append([]byte{}, authData...), hash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return mustJSON(t, map[string]any{
		"id": b64.EncodeToString(a.id), "rawId": b64.EncodeToString(a.id), "type": "public-key",
		"response": map[string]string{
			"clientDataJSON":    b64.EncodeToString(clientData),
			"authenticatorData": b64.EncodeToString(authData),
			"signature":         b64.EncodeToString(sig),
		},
	})
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// testAuth is an Auth with passkeys on, behind the same routes the server
// mounts.
type testAuth struct {
	*Auth
	h  http.Handler
	db *store.Store
}

func newTestAuth(t *testing.T, allowToken bool) *testAuth {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tessaly.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	a := New(strings.Repeat("A", tokenLen))
	if err := a.EnablePasskeys(db, testPort, allowToken); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", a.LoginOptions)
	mux.HandleFunc("POST /login", a.Login)
	mux.HandleFunc("POST /login/passkey/begin", a.PasskeyLoginBegin)
	mux.HandleFunc("POST /login/passkey/finish", a.PasskeyLoginFinish)
	api := http.NewServeMux()
	api.HandleFunc("GET /api/passkeys", a.ListPasskeys)
	api.HandleFunc("POST /api/passkeys/register/begin", a.RegisterBegin)
	api.HandleFunc("POST /api/passkeys/register/finish", a.RegisterFinish)
	api.HandleFunc("POST /api/passkeys/verify/begin", a.VerifyBegin)
	api.HandleFunc("POST /api/passkeys/verify/finish", a.VerifyFinish)
	api.HandleFunc("DELETE /api/passkeys/{id}", a.DeletePasskey)
	api.Handle("POST /api/fresh", a.RequireFresh(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
	mux.Handle("/api/", a.Require(api))
	return &testAuth{Auth: a, h: mux, db: db}
}

// client is one browser: a cookie and a session key.
type client struct {
	cookie *http.Cookie
	key    string
	host   string
}

func (ta *testAuth) do(t *testing.T, c *client, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, bytes.NewReader(mustJSON(t, body)))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Host = testHost
	if c != nil {
		if c.host != "" {
			r.Host = c.host
		}
		if c.cookie != nil {
			r.AddCookie(c.cookie)
			r.Header.Set(KeyHeader, c.key)
		}
	}
	rec := httptest.NewRecorder()
	ta.h.ServeHTTP(rec, r)
	return rec
}

// signedIn reads the session from a sign-in reply.
func signedIn(t *testing.T, rec *httptest.ResponseRecorder) *client {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("sign-in = %d %s, want 200", rec.Code, rec.Body)
	}
	var body struct{ Key string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == CookieName {
			return &client{cookie: ck, key: body.Key}
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func (ta *testAuth) tokenLogin(t *testing.T) *httptest.ResponseRecorder {
	return ta.do(t, nil, "POST", "/login", map[string]string{"token": strings.Repeat("A", tokenLen)})
}

type begun struct {
	Ceremony string          `json:"ceremony"`
	Options  json.RawMessage `json:"options"`
}

func (ta *testAuth) begin(t *testing.T, c *client, target string) begun {
	t.Helper()
	rec := ta.do(t, c, "POST", target, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s = %d %s, want 200", target, rec.Code, rec.Body)
	}
	var b begun
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	return b
}

// register adds dev as a passkey from the signed-in client c.
func (ta *testAuth) register(t *testing.T, c *client, dev *authenticator) *httptest.ResponseRecorder {
	t.Helper()
	b := ta.begin(t, c, "/api/passkeys/register/begin")
	return ta.do(t, c, "POST", "/api/passkeys/register/finish", map[string]any{
		"ceremony": b.Ceremony, "name": "Laptop", "credential": dev.create(t, b.Options),
	})
}

func (ta *testAuth) passkeyLogin(t *testing.T, dev *authenticator) *httptest.ResponseRecorder {
	t.Helper()
	b := ta.begin(t, nil, "/login/passkey/begin")
	return ta.do(t, nil, "POST", "/login/passkey/finish", map[string]any{
		"ceremony": b.Ceremony, "credential": dev.get(t, b.Options),
	})
}

func (ta *testAuth) loginOptions(t *testing.T) (token, passkey bool) {
	t.Helper()
	rec := ta.do(t, nil, "GET", "/login", nil)
	var body struct {
		Token, Passkey bool
		PasskeyOrigin  string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.PasskeyOrigin != testOrigin {
		t.Errorf("passkeyOrigin = %q, want %q", body.PasskeyOrigin, testOrigin)
	}
	return body.Token, body.Passkey
}

func TestPasskeyRegisterAndSignIn(t *testing.T) {
	ta := newTestAuth(t, false)
	if token, passkey := ta.loginOptions(t); !token || passkey {
		t.Errorf("before any passkey: token %v passkey %v, want true false", token, passkey)
	}

	// Signing in with a passkey needs one to exist.
	if rec := ta.do(t, nil, "POST", "/login/passkey/begin", nil); rec.Code != http.StatusConflict {
		t.Errorf("passkey begin with none = %d, want 409", rec.Code)
	}

	c := signedIn(t, ta.tokenLogin(t))
	dev := newAuthenticator(t)
	if rec := ta.register(t, c, dev); rec.Code != http.StatusCreated {
		t.Fatalf("register = %d %s, want 201", rec.Code, rec.Body)
	}

	// Token sign-in turns off once a passkey exists.
	if token, passkey := ta.loginOptions(t); token || !passkey {
		t.Errorf("after a passkey: token %v passkey %v, want false true", token, passkey)
	}
	if rec := ta.tokenLogin(t); rec.Code != http.StatusForbidden {
		t.Errorf("token sign-in after a passkey = %d, want 403", rec.Code)
	}
	// A wrong token gets the same answer, so a closed sign-in tells nobody
	// whether a token is right.
	if rec := ta.do(t, nil, "POST", "/login", map[string]string{"token": "wrong"}); rec.Code != http.StatusForbidden {
		t.Errorf("wrong token after a passkey = %d, want 403", rec.Code)
	}
	// The session that was already signed in keeps working.
	if rec := ta.do(t, c, "GET", "/api/passkeys", nil); rec.Code != http.StatusOK {
		t.Errorf("old session = %d, want 200", rec.Code)
	}

	pc := signedIn(t, ta.passkeyLogin(t, dev))
	rec := ta.do(t, pc, "GET", "/api/passkeys", nil)
	var list struct {
		Passkeys []PasskeyInfo
		Fresh    bool
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Passkeys) != 1 || list.Passkeys[0].Name != "Laptop" || list.Passkeys[0].LastUsedAt == nil {
		t.Errorf("passkeys = %+v, want one used passkey named Laptop", list.Passkeys)
	}
	if !list.Fresh {
		t.Error("a passkey sign-in should count as a fresh check")
	}
}

func TestAllowTokenLoginKeepsTokenOn(t *testing.T) {
	ta := newTestAuth(t, true)
	c := signedIn(t, ta.tokenLogin(t))
	if rec := ta.register(t, c, newAuthenticator(t)); rec.Code != http.StatusCreated {
		t.Fatalf("register = %d", rec.Code)
	}
	if token, _ := ta.loginOptions(t); !token {
		t.Error("token sign-in should stay on with --allow-token-login")
	}
	signedIn(t, ta.tokenLogin(t))
}

func TestPasskeysNeedLocalhost(t *testing.T) {
	ta := newTestAuth(t, false)
	ip := &client{host: "127.0.0.1:7777"}
	rec := ta.do(t, ip, "POST", "/login/passkey/begin", nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), testOrigin) {
		t.Errorf("begin on 127.0.0.1 = %d %s, want 400 naming %s", rec.Code, rec.Body, testOrigin)
	}
}

func TestPasskeySignInRefuses(t *testing.T) {
	ta := newTestAuth(t, false)
	c := signedIn(t, ta.tokenLogin(t))
	dev := newAuthenticator(t)
	if rec := ta.register(t, c, dev); rec.Code != http.StatusCreated {
		t.Fatalf("register = %d", rec.Code)
	}

	t.Run("another origin", func(t *testing.T) {
		// A local server on another port could ask the browser to sign
		// for "localhost". Its origin gives it away.
		evil := *dev
		evil.origin = "http://localhost:3000"
		if rec := ta.passkeyLogin(t, &evil); rec.Code != http.StatusUnauthorized {
			t.Errorf("= %d, want 401", rec.Code)
		}
		dev.counter = evil.counter
	})
	t.Run("no user verification", func(t *testing.T) {
		weak := *dev
		weak.noUV = true
		if rec := ta.passkeyLogin(t, &weak); rec.Code != http.StatusUnauthorized {
			t.Errorf("= %d, want 401", rec.Code)
		}
		dev.counter = weak.counter
	})
	t.Run("unknown passkey", func(t *testing.T) {
		if rec := ta.passkeyLogin(t, newAuthenticator(t)); rec.Code != http.StatusUnauthorized {
			t.Errorf("= %d, want 401", rec.Code)
		}
	})
	t.Run("counter goes backwards", func(t *testing.T) {
		signedIn(t, ta.passkeyLogin(t, dev))
		clone := *dev
		clone.counter -= 2
		if rec := ta.passkeyLogin(t, &clone); rec.Code != http.StatusUnauthorized {
			t.Errorf("= %d, want 401", rec.Code)
		}
	})
	t.Run("ceremony used twice", func(t *testing.T) {
		b := ta.begin(t, nil, "/login/passkey/begin")
		body := map[string]any{"ceremony": b.Ceremony, "credential": dev.get(t, b.Options)}
		signedIn(t, ta.do(t, nil, "POST", "/login/passkey/finish", body))
		if rec := ta.do(t, nil, "POST", "/login/passkey/finish", body); rec.Code != http.StatusBadRequest {
			t.Errorf("replay = %d, want 400", rec.Code)
		}
	})
	t.Run("ceremony of another kind", func(t *testing.T) {
		// A sign-in ceremony cannot finish a passkey check, which is tied
		// to a session.
		b := ta.begin(t, nil, "/login/passkey/begin")
		rec := ta.do(t, c, "POST", "/api/passkeys/verify/finish",
			map[string]any{"ceremony": b.Ceremony, "credential": dev.get(t, b.Options)})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("= %d, want 400", rec.Code)
		}
	})
	t.Run("expired ceremony", func(t *testing.T) {
		b := ta.begin(t, nil, "/login/passkey/begin")
		ta.now = func() time.Time { return time.Now().Add(ceremonyTTL) }
		defer func() { ta.now = time.Now }()
		rec := ta.do(t, nil, "POST", "/login/passkey/finish",
			map[string]any{"ceremony": b.Ceremony, "credential": dev.get(t, b.Options)})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("= %d, want 400", rec.Code)
		}
	})
}

func TestPasskeyFailuresCountTowardTheLimit(t *testing.T) {
	ta := newTestAuth(t, false)
	c := signedIn(t, ta.tokenLogin(t))
	dev := newAuthenticator(t)
	if rec := ta.register(t, c, dev); rec.Code != http.StatusCreated {
		t.Fatalf("register = %d", rec.Code)
	}
	for range MaxFailedLogins {
		if rec := ta.passkeyLogin(t, newAuthenticator(t)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong passkey = %d, want 401", rec.Code)
		}
	}
	if rec := ta.do(t, nil, "POST", "/login/passkey/begin", nil); rec.Code != http.StatusTooManyRequests {
		t.Errorf("after %d failures = %d, want 429", MaxFailedLogins, rec.Code)
	}
}

func TestChangingPasskeysNeedsAFreshCheck(t *testing.T) {
	ta := newTestAuth(t, true)
	token := signedIn(t, ta.tokenLogin(t))
	dev := newAuthenticator(t)
	if rec := ta.register(t, token, dev); rec.Code != http.StatusCreated {
		t.Fatalf("first register = %d", rec.Code)
	}

	// A fresh token session cannot add a second passkey or delete one.
	other := signedIn(t, ta.tokenLogin(t))
	if rec := ta.do(t, other, "POST", "/api/passkeys/register/begin", nil); rec.Code != http.StatusForbidden {
		t.Errorf("second register without a check = %d, want 403", rec.Code)
	}
	if rec := ta.do(t, other, "DELETE", "/api/passkeys/1", nil); rec.Code != http.StatusForbidden {
		t.Errorf("delete without a check = %d, want 403", rec.Code)
	}
	if rec := ta.do(t, other, "POST", "/api/fresh", nil); rec.Code != http.StatusForbidden {
		t.Errorf("RequireFresh without a check = %d, want 403", rec.Code)
	}

	// A passkey check opens them up, for FreshFor.
	b := ta.begin(t, other, "/api/passkeys/verify/begin")
	rec := ta.do(t, other, "POST", "/api/passkeys/verify/finish",
		map[string]any{"ceremony": b.Ceremony, "credential": dev.get(t, b.Options)})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("verify = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := ta.do(t, other, "POST", "/api/fresh", nil); rec.Code != http.StatusNoContent {
		t.Errorf("RequireFresh after a check = %d, want 204", rec.Code)
	}
	if rec := ta.register(t, other, newAuthenticator(t)); rec.Code != http.StatusCreated {
		t.Errorf("second register after a check = %d, want 201", rec.Code)
	}
	ta.now = func() time.Time { return time.Now().Add(FreshFor) }
	if rec := ta.do(t, other, "POST", "/api/fresh", nil); rec.Code != http.StatusForbidden {
		t.Errorf("RequireFresh after FreshFor = %d, want 403", rec.Code)
	}
	ta.now = time.Now

	// A check started by one session cannot finish in another.
	b = ta.begin(t, other, "/api/passkeys/verify/begin")
	rec = ta.do(t, token, "POST", "/api/passkeys/verify/finish",
		map[string]any{"ceremony": b.Ceremony, "credential": dev.get(t, b.Options)})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("verify from another session = %d, want 400", rec.Code)
	}
}

func TestDeletingAPasskeyEndsItsSessions(t *testing.T) {
	ta := newTestAuth(t, true)
	token := signedIn(t, ta.tokenLogin(t))
	dev := newAuthenticator(t)
	if rec := ta.register(t, token, dev); rec.Code != http.StatusCreated {
		t.Fatalf("register = %d", rec.Code)
	}
	pc := signedIn(t, ta.passkeyLogin(t, dev))

	if rec := ta.do(t, pc, "DELETE", "/api/passkeys/9", nil); rec.Code != http.StatusNotFound {
		t.Errorf("delete unknown = %d, want 404", rec.Code)
	}
	if rec := ta.do(t, pc, "DELETE", "/api/passkeys/x", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("delete bad id = %d, want 400", rec.Code)
	}
	if rec := ta.do(t, pc, "DELETE", "/api/passkeys/1", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := ta.do(t, pc, "GET", "/api/passkeys", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("session of the deleted passkey = %d, want 401", rec.Code)
	}
	if rec := ta.do(t, token, "GET", "/api/passkeys", nil); rec.Code != http.StatusOK {
		t.Errorf("token session = %d, want 200", rec.Code)
	}
	if token, passkey := ta.loginOptions(t); !token || passkey {
		t.Errorf("after the last passkey: token %v passkey %v, want true false", token, passkey)
	}
}

func TestRegisterRefuses(t *testing.T) {
	ta := newTestAuth(t, false)
	c := signedIn(t, ta.tokenLogin(t))
	dev := newAuthenticator(t)

	b := ta.begin(t, c, "/api/passkeys/register/begin")
	long := strings.Repeat("x", maxPasskeyName+1)
	for _, name := range []string{long, "bad\nname"} {
		rec := ta.do(t, c, "POST", "/api/passkeys/register/finish",
			map[string]any{"ceremony": b.Ceremony, "name": name, "credential": dev.create(t, b.Options)})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name %q = %d, want 400", name, rec.Code)
		}
	}

	b = ta.begin(t, c, "/api/passkeys/register/begin")
	evil := *dev
	evil.origin = "http://localhost:3000"
	rec := ta.do(t, c, "POST", "/api/passkeys/register/finish",
		map[string]any{"ceremony": b.Ceremony, "credential": evil.create(t, b.Options)})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("another origin = %d, want 400", rec.Code)
	}

	for _, body := range []string{`{`, `{"ceremony":"x","credential":{}}`, `{"ceremony":"` + strings.Repeat("A", tokenLen) + `","credential":{},"extra":1}`} {
		r := httptest.NewRequest("POST", "/api/passkeys/register/finish", strings.NewReader(body))
		r.Host = testHost
		r.AddCookie(c.cookie)
		r.Header.Set(KeyHeader, c.key)
		rec := httptest.NewRecorder()
		ta.h.ServeHTTP(rec, r)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s = %d, want 400", body, rec.Code)
		}
	}
}

func TestCeremoniesAreCapped(t *testing.T) {
	ta := newTestAuth(t, false)
	c := signedIn(t, ta.tokenLogin(t))
	for range maxCeremonies {
		ta.begin(t, c, "/api/passkeys/register/begin")
	}
	if rec := ta.do(t, c, "POST", "/api/passkeys/register/begin", nil); rec.Code != http.StatusTooManyRequests {
		t.Errorf("one past the cap = %d, want 429", rec.Code)
	}
	// Expired prompts make room again.
	ta.now = func() time.Time { return time.Now().Add(ceremonyTTL) }
	defer func() { ta.now = time.Now }()
	if rec := ta.do(t, c, "POST", "/api/passkeys/register/begin", nil); rec.Code != http.StatusOK {
		t.Errorf("after expiry = %d, want 200", rec.Code)
	}
}

func TestPasskeyName(t *testing.T) {
	for in, want := range map[string]string{"": "Passkey", "  Phone ": "Phone", "Ünïcode ✓": "Ünïcode ✓"} {
		if got, err := passkeyName(in); err != nil || got != want {
			t.Errorf("passkeyName(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
}
