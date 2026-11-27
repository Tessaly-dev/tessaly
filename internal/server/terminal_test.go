// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/tessaly-dev/tessaly/internal/auth"
	"github.com/tessaly-dev/tessaly/internal/config"
	"github.com/tessaly-dev/tessaly/internal/store"
)

// startServer runs the handler on a real listener with short timeouts, so
// tests catch a stream that the server's timeouts would cut. It returns the
// server and the handler, for plain requests through send.
func startServer(t *testing.T, agents map[string]config.Agent) (*httptest.Server, http.Handler) {
	t.Helper()
	cfg := testConfig(t, testPort, testUI)
	if agents != nil {
		cfg.Agents = agents
	}
	h := Handler(cfg)
	// The listener has a random port, but the handler expects testPort in
	// the Host header. Rewrite it, as a reverse proxy would.
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Host = "127.0.0.1:7777"
		h.ServeHTTP(w, r)
	}))
	srv.Config.WriteTimeout = 300 * time.Millisecond
	srv.Config.ReadTimeout = 300 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, h
}

// sameOrigin is the Origin a browser sends for the UI at 127.0.0.1:7777.
const sameOrigin = "http://127.0.0.1:7777"

// dial opens the terminal socket for ws the way the UI does: the cookie in
// the Cookie header and the key as a subprotocol. It returns the handshake
// status code, or 0 when there was no reply.
func dial(ctx context.Context, srv *httptest.Server, ws store.Workspace, c *creds, origin string) (*websocket.Conn, int, error) {
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + wsPath(ws, "terminal")
	h := http.Header{}
	opts := &websocket.DialOptions{HTTPHeader: h}
	if c != nil {
		if c.cookie != nil {
			h.Set("Cookie", c.cookie.String())
		}
		opts.Subprotocols = []string{auth.Protocol, auth.KeyProtocol + c.key}
	}
	if origin != "" {
		h.Set("Origin", origin)
	}
	conn, resp, err := websocket.Dial(ctx, url, opts)
	if resp == nil {
		return conn, 0, err
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	return conn, resp.StatusCode, err
}

// readUntil reads binary messages until the text holds want.
func readUntil(ctx context.Context, t *testing.T, conn *websocket.Conn, want string) string {
	t.Helper()
	var got strings.Builder
	for !strings.Contains(got.String(), want) {
		_, b, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v (so far %q)", err, got.String())
		}
		got.Write(b)
	}
	return got.String()
}

func TestTerminalStreamsAndTakesInput(t *testing.T) {
	srv, h := startServer(t, map[string]config.Agent{"cat": {Command: []string{"cat"}}})
	c := signIn(t, h)
	ws := newWorkspace(t, h, c, "term")
	if rec := send(t, h, http.MethodPost, wsPath(ws, "start"), startBody("cat", ""), c, nil); rec.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", rec.Code, rec.Body)
	}
	t.Cleanup(func() { send(t, h, http.MethodPost, wsPath(ws, "stop"), "", c, nil) })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := dial(ctx, srv, ws, c, sameOrigin)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()
	if got := conn.Subprotocol(); got != auth.Protocol {
		t.Errorf("subprotocol = %q, want %q, so the key is not echoed", got, auth.Protocol)
	}

	resize, _ := json.Marshal(terminalMessage{Type: "resize", Rows: 40, Cols: 100})
	if err := conn.Write(ctx, websocket.MessageText, resize); err != nil {
		t.Fatal(err)
	}
	// Wait past the server's timeouts, then check the stream still works.
	<-time.After(500 * time.Millisecond)
	input, _ := json.Marshal(terminalMessage{Type: "input", Data: "hello pty\n"})
	if err := conn.Write(ctx, websocket.MessageText, input); err != nil {
		t.Fatal(err)
	}
	readUntil(ctx, t, conn, "hello pty")
}

func TestTerminalAfterExitSendsTranscript(t *testing.T) {
	srv, h := startServer(t, nil)
	c := signIn(t, h)
	ws := newWorkspace(t, h, c, "done")
	send(t, h, http.MethodPost, wsPath(ws, "start"), startBody("echo", "finished work"), c, nil)
	waitState(t, h, c, ws.ID, "exited")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := dial(ctx, srv, ws, c, sameOrigin)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()
	readUntil(ctx, t, conn, "said: finished work")
	if _, _, err := conn.Read(ctx); websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Errorf("close = %v, want normal closure", err)
	}
}

func TestTerminalRejects(t *testing.T) {
	srv, h := startServer(t, nil)
	c := signIn(t, h)
	ws := newWorkspace(t, h, c, "guarded")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tests := []struct {
		name   string
		cookie *creds
		origin string
		want   int
	}{
		{"no session", nil, sameOrigin, http.StatusUnauthorized},
		{"cookie without key", &creds{cookie: c.cookie}, sameOrigin, http.StatusUnauthorized},
		{"key without cookie", &creds{key: c.key}, sameOrigin, http.StatusUnauthorized},
		{"other site", c, "http://evil.example", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, status, err := dial(ctx, srv, ws, tt.cookie, tt.origin)
			if err == nil {
				_ = conn.CloseNow()
				t.Fatal("dial succeeded")
			}
			if status != tt.want {
				t.Errorf("status = %d, want %d", status, tt.want)
			}
		})
	}
}
