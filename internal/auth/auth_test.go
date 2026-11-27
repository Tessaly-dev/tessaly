// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadOrCreateToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")

	first, err := LoadOrCreateToken(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(first) != tokenLen {
		t.Errorf("token length = %d, want %d", len(first), tokenLen)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file mode = %04o, want 0600", perm)
	}

	second, err := LoadOrCreateToken(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if second != first {
		t.Error("second load returned a different token")
	}
}

func TestLoadOrCreateTokenRejectsUnsafeFiles(t *testing.T) {
	dir := t.TempDir()
	good := strings.Repeat("A", tokenLen) + "\n"

	readable := filepath.Join(dir, "readable")
	if err := os.WriteFile(readable, []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readable, 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "link")
	if err := os.Symlink(readable, link); err != nil {
		t.Fatal(err)
	}

	malformed := filepath.Join(dir, "malformed")
	if err := os.WriteFile(malformed, []byte("short\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, path, wantErr string
	}{
		{"group or world readable", readable, "other users can read it"},
		{"symlink", link, "not a regular file"},
		{"malformed", malformed, "malformed"},
		{"folder", dir, "not a regular file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadOrCreateToken(tt.path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewSecretIsRandom(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		s := newSecret()
		if len(s) != tokenLen {
			t.Fatalf("secret length = %d, want %d", len(s), tokenLen)
		}
		if seen[s] {
			t.Fatal("newSecret repeated a value")
		}
		seen[s] = true
	}
}

func login(t *testing.T, a *Auth, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.Login(rec, req)
	return rec
}

func loginKey(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Key) != tokenLen {
		t.Fatalf("login body %q has no key: %v", rec.Body.String(), err)
	}
	return body.Key
}

func TestRequestKey(t *testing.T) {
	key := strings.Repeat("K", tokenLen)
	tests := []struct {
		name   string
		header http.Header
		want   string
	}{
		{"none", http.Header{}, ""},
		{"header", http.Header{"X-Tessaly-Key": {key}}, key},
		{"subprotocol", http.Header{"Sec-Websocket-Protocol": {"tessaly, " + KeyProtocol + key}}, key},
		{"subprotocol in its own header", http.Header{"Sec-Websocket-Protocol": {"tessaly", KeyProtocol + key}}, key},
		{"plain subprotocol only", http.Header{"Sec-Websocket-Protocol": {"tessaly"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header = tt.header
			if got := requestKey(req); got != tt.want {
				t.Errorf("requestKey = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRequireNeedsCookieAndKey(t *testing.T) {
	token := strings.Repeat("A", tokenLen)
	a := New(token)
	first := login(t, a, `{"token":"`+token+`"}`)
	second := login(t, a, `{"token":"`+token+`"}`)
	cookie, key := first.Result().Cookies()[0], loginKey(t, first)
	otherKey := loginKey(t, second)

	protected := a.Require(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tests := []struct {
		name   string
		cookie *http.Cookie
		key    string
		want   int
	}{
		{"both", cookie, key, http.StatusOK},
		{"cookie only", cookie, "", http.StatusUnauthorized},
		{"key only", nil, key, http.StatusUnauthorized},
		{"key of another session", cookie, otherKey, http.StatusUnauthorized},
		{"short key", cookie, key[:10], http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			if tt.key != "" {
				req.Header.Set(KeyHeader, tt.key)
			}
			rec := httptest.NewRecorder()
			protected.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestLoginRejectsBadBodies(t *testing.T) {
	a := New(strings.Repeat("A", tokenLen))
	tests := []struct {
		name, body string
		want       int
	}{
		{"not json", "token=x", http.StatusBadRequest},
		{"too large", `{"token":"` + strings.Repeat("A", maxLoginBody) + `"}`, http.StatusBadRequest},
		{"empty token", `{"token":""}`, http.StatusUnauthorized},
		{"wrong token", `{"token":"` + strings.Repeat("B", tokenLen) + `"}`, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := login(t, a, tt.body); rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestSessionExpires(t *testing.T) {
	token := strings.Repeat("A", tokenLen)
	a := New(token)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }

	rec := login(t, a, `{"token":"`+token+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	key := loginKey(t, rec)

	protected := a.Require(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	call := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
		req.AddCookie(cookies[0])
		req.Header.Set(KeyHeader, key)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := call(); got != http.StatusOK {
		t.Fatalf("fresh session = %d, want 200", got)
	}
	now = now.Add(SessionTTL - time.Second)
	if got := call(); got != http.StatusOK {
		t.Fatalf("session just before expiry = %d, want 200", got)
	}
	now = now.Add(time.Second)
	if got := call(); got != http.StatusUnauthorized {
		t.Fatalf("expired session = %d, want 401", got)
	}
	if len(a.sessions) != 0 {
		t.Errorf("expired session still stored")
	}
}

func TestLoginLimitsWrongTokens(t *testing.T) {
	token := strings.Repeat("A", tokenLen)
	a := New(token)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	wrong := `{"token":"` + strings.Repeat("B", tokenLen) + `"}`
	right := `{"token":"` + token + `"}`

	for i := range MaxFailedLogins {
		if rec := login(t, a, wrong); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong token %d = %d, want 401", i, rec.Code)
		}
	}
	// Sign-in is closed now, even for the right token.
	now = now.Add(10 * time.Second)
	rec := login(t, a, right)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("login over the limit = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "50" {
		t.Errorf("Retry-After = %q, want 50", got)
	}

	// It opens again once the window ends.
	now = now.Add(FailedLoginWindow)
	if rec := login(t, a, right); rec.Code != http.StatusOK {
		t.Fatalf("login after the window = %d, want 200", rec.Code)
	}
}

func TestLoginRightTokensAreNotLimited(t *testing.T) {
	token := strings.Repeat("A", tokenLen)
	a := New(token)
	for i := range MaxFailedLogins * 2 {
		if rec := login(t, a, `{"token":"`+token+`"}`); rec.Code != http.StatusOK {
			t.Fatalf("login %d = %d, want 200", i, rec.Code)
		}
	}
}
