// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tessaly-dev/tessaly/internal/version"
)

const testPort = 7777

var testUI = fstest.MapFS{
	"index.html":     {Data: []byte("<!doctype html><title>Tessaly</title>")},
	"assets/app.js":  {Data: []byte("console.log('hi')")},
	".gitkeep":       {Data: []byte("")},
	"assets/.secret": {Data: []byte("x")},
	".hidden/app.js": {Data: []byte("x")},
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
	h := Handler(testUI, testPort)

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
	h := Handler(testUI, testPort)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := do(t, h, m, "/healthz", "127.0.0.1:7777")
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /healthz status = %d, want 405", m, rec.Code)
		}
	}
}

func TestHostCheck(t *testing.T) {
	h := Handler(testUI, testPort)
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
	h := Handler(testUI, testPort)
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
	h := Handler(testUI, testPort)
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
	h := Handler(fstest.MapFS{".gitkeep": {}}, testPort)
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
	go func() { done <- Serve(ctx, ln, Handler(testUI, port)) }()

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
