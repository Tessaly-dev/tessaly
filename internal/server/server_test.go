// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tessaly-dev/tessaly/internal/auth"
	"github.com/tessaly-dev/tessaly/internal/config"
	"github.com/tessaly-dev/tessaly/internal/store"
	"github.com/tessaly-dev/tessaly/internal/version"
	"github.com/tessaly-dev/tessaly/internal/workspace"
)

const testPort = 7777

var testUI = fstest.MapFS{
	"index.html":     {Data: []byte("<!doctype html><title>Tessaly</title>")},
	"assets/app.js":  {Data: []byte("console.log('hi')")},
	".gitkeep":       {Data: []byte("")},
	"assets/.secret": {Data: []byte("x")},
	".hidden/app.js": {Data: []byte("x")},
}

// testToken is a well-formed token: 43 base64url characters.
const testToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func testHandler(t *testing.T, ui fstest.MapFS) http.Handler {
	t.Helper()
	return Handler(testConfig(t, testPort, ui))
}

func testConfig(t *testing.T, port int, ui fstest.MapFS) Config {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tessaly.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return Config{
		Port:       port,
		UI:         ui,
		Auth:       auth.New(testToken),
		Store:      db,
		Workspaces: workspace.NewManager(db, t.TempDir()),
		Agents: map[string]config.Agent{
			"echo":  {Command: []string{"printf", "said: %s\\n", "{prompt}"}},
			"sleep": {Command: []string{"sleep", "60"}},
		},
	}
}

func do(t *testing.T, h http.Handler, method, target, host string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAddr(t *testing.T) {
	tests := []struct {
		port    int
		want    string
		wantErr bool
	}{
		{7777, "127.0.0.1:7777", false},
		{1, "127.0.0.1:1", false},
		{65535, "127.0.0.1:65535", false},
		{0, "", true},
		{-1, "", true},
		{65536, "", true},
	}
	for _, tt := range tests {
		got, err := Addr(tt.port)
		if (err != nil) != tt.wantErr {
			t.Errorf("Addr(%d) error = %v, wantErr %v", tt.port, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("Addr(%d) = %q, want %q", tt.port, got, tt.want)
		}
	}
}

func TestHealth(t *testing.T) {
	h := testHandler(t, testUI)

	rec := do(t, h, http.MethodGet, "/healthz", "127.0.0.1:7777")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" || body.Version != version.Version {
		t.Errorf("body = %+v, want status ok and version %q", body, version.Version)
	}
}

func TestHealthRejectsOtherMethods(t *testing.T) {
	h := testHandler(t, testUI)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := do(t, h, m, "/healthz", "127.0.0.1:7777")
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /healthz status = %d, want 405", m, rec.Code)
		}
	}
}

func TestHostCheck(t *testing.T) {
	h := testHandler(t, testUI)
	tests := []struct {
		host string
		want int
	}{
		{"127.0.0.1:7777", http.StatusOK},
		{"localhost:7777", http.StatusOK},
		{"LOCALHOST:7777", http.StatusOK},
		{"evil.example:7777", http.StatusForbidden},
		{"127.0.0.1", http.StatusForbidden},
		{"127.0.0.1:8080", http.StatusForbidden},
		{"0.0.0.0:7777", http.StatusForbidden},
		{"", http.StatusForbidden},
	}
	for _, tt := range tests {
		rec := do(t, h, http.MethodGet, "/healthz", tt.host)
		if rec.Code != tt.want {
			t.Errorf("Host %q status = %d, want %d", tt.host, rec.Code, tt.want)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := testHandler(t, testUI)
	// Headers must be set on rejected requests too.
	for _, host := range []string{"127.0.0.1:7777", "evil.example"} {
		rec := do(t, h, http.MethodGet, "/", host)
		for _, name := range []string{
			"Content-Security-Policy",
			"X-Content-Type-Options",
			"X-Frame-Options",
			"Referrer-Policy",
		} {
			if rec.Header().Get(name) == "" {
				t.Errorf("Host %q: missing header %s", host, name)
			}
		}
	}
}

func TestUI(t *testing.T) {
	h := testHandler(t, testUI)
	tests := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/", http.StatusOK},
		{http.MethodHead, "/", http.StatusOK},
		{http.MethodGet, "/assets/app.js", http.StatusOK},
		{http.MethodGet, "/missing.js", http.StatusNotFound},
		{http.MethodGet, "/assets/", http.StatusNotFound},
		{http.MethodGet, "/assets", http.StatusNotFound},
		{http.MethodGet, "/.gitkeep", http.StatusNotFound},
		{http.MethodGet, "/assets/.secret", http.StatusNotFound},
		{http.MethodGet, "/.hidden/app.js", http.StatusNotFound},
		{http.MethodPost, "/", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		rec := do(t, h, tt.method, tt.path, "127.0.0.1:7777")
		if rec.Code != tt.want {
			t.Errorf("%s %s status = %d, want %d", tt.method, tt.path, rec.Code, tt.want)
		}
	}
}

func TestUIPathTraversal(t *testing.T) {
	// The mux cleans paths first, so call the UI handler directly.
	h := uiHandler(testUI)
	for _, p := range []string{"/../../etc/passwd", "/assets/../../index.html", "/..%2f..%2fetc/passwd"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = p
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK && rec.Body.String() != string(testUI["index.html"].Data) {
			t.Errorf("path %q served unexpected content", p)
		}
		switch rec.Code {
		case http.StatusOK, http.StatusBadRequest, http.StatusNotFound:
		default:
			t.Errorf("path %q status = %d, want 200 (index), 400 or 404", p, rec.Code)
		}
	}
}

func TestUINotBuilt(t *testing.T) {
	h := testHandler(t, fstest.MapFS{".gitkeep": {}})
	rec := do(t, h, http.MethodGet, "/", "127.0.0.1:7777")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestServe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, Handler(testConfig(t, port, testUI))) }()

	resp, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not stop after cancel")
	}
}

// creds holds the two halves of a sign-in. Either may be empty, to test
// requests that carry only one.
type creds struct {
	cookie *http.Cookie
	key    string
}

// send makes a request with an optional JSON body, session and extra headers.
func send(t *testing.T, h http.Handler, method, target, body string, s *creds, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = "127.0.0.1:7777"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if s != nil && s.cookie != nil {
		req.AddCookie(s.cookie)
	}
	if s != nil && s.key != "" {
		req.Header.Set(auth.KeyHeader, s.key)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// credsFrom reads the cookie and key from a login reply.
func credsFrom(t *testing.T, rec *httptest.ResponseRecorder) *creds {
	t.Helper()
	var body struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Key == "" {
		t.Fatalf("login body %q has no key: %v", rec.Body.String(), err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return &creds{cookie: c, key: body.Key}
		}
	}
	t.Fatalf("no %s cookie in response", auth.CookieName)
	return nil
}

func TestAPINeedsSignIn(t *testing.T) {
	h := testHandler(t, testUI)
	for _, p := range []string{"/api/v1/session", "/api/v1/unknown", "/api/"} {
		if rec := send(t, h, http.MethodGet, p, "", nil, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without session = %d, want 401", p, rec.Code)
		}
	}
	bogus := &creds{cookie: &http.Cookie{Name: auth.CookieName, Value: testToken}, key: testToken}
	if rec := send(t, h, http.MethodGet, "/api/v1/session", "", bogus, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/session with unknown session = %d, want 401", rec.Code)
	}
}

func TestAPILoginFlow(t *testing.T) {
	h := testHandler(t, testUI)

	if rec := send(t, h, http.MethodPost, "/api/v1/login", `{"token":"wrong"}`, nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("login with wrong token = %d, want 401", rec.Code)
	}

	rec := send(t, h, http.MethodPost, "/api/v1/login", `{"token":"`+testToken+`"}`, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("login Cache-Control = %q, want no-store", got)
	}
	c := credsFrom(t, rec)
	if !c.cookie.HttpOnly || c.cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie HttpOnly=%v SameSite=%v, want HttpOnly and Strict", c.cookie.HttpOnly, c.cookie.SameSite)
	}
	if c.key == c.cookie.Value {
		t.Error("session key equals the cookie value")
	}

	// A local server on another port gets the cookie but never the key,
	// so the cookie alone must not be enough. Nor is the key alone.
	halves := map[string]*creds{
		"cookie only": {cookie: c.cookie},
		"key only":    {key: c.key},
		"wrong key":   {cookie: c.cookie, key: testToken},
	}
	for name, s := range halves {
		if rec := send(t, h, http.MethodGet, "/api/v1/session", "", s, nil); rec.Code != http.StatusUnauthorized {
			t.Errorf("session with %s = %d, want 401", name, rec.Code)
		}
	}

	if rec := send(t, h, http.MethodGet, "/api/v1/session", "", c, nil); rec.Code != http.StatusOK {
		t.Errorf("session after login = %d, want 200", rec.Code)
	}
	if rec := send(t, h, http.MethodGet, "/api/v1/unknown", "", c, nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown route after login = %d, want 404", rec.Code)
	}

	// The cookie alone cannot end the session.
	send(t, h, http.MethodPost, "/api/v1/logout", "", &creds{cookie: c.cookie}, nil)
	if rec := send(t, h, http.MethodGet, "/api/v1/session", "", c, nil); rec.Code != http.StatusOK {
		t.Errorf("session after cookie-only logout = %d, want 200", rec.Code)
	}

	if rec := send(t, h, http.MethodPost, "/api/v1/logout", "", c, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", rec.Code)
	}
	if rec := send(t, h, http.MethodGet, "/api/v1/session", "", c, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("session after logout = %d, want 401", rec.Code)
	}
}

func TestAPIRejectsCrossSiteRequests(t *testing.T) {
	h := testHandler(t, testUI)
	body := `{"token":"` + testToken + `"}`
	tests := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"same origin", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusOK},
		{"cross site fetch", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"same site, other port", map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{"foreign Origin", map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		{"matching Origin", map[string]string{"Origin": "http://127.0.0.1:7777"}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := send(t, h, http.MethodPost, "/api/v1/login", body, nil, tt.headers)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestCSPKeepsScriptsStrict(t *testing.T) {
	rec := do(t, testHandler(t, testUI), http.MethodGet, "/", "127.0.0.1:7777")
	got := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"script-src 'self';", "frame-ancestors 'none'", "object-src 'none'"} {
		if !strings.Contains(got, want) {
			t.Errorf("CSP %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "'unsafe-eval'") || strings.Contains(got, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("CSP %q allows inline or eval scripts", got)
	}
}
