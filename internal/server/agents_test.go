// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tessaly-dev/tessaly/internal/store"
)

func newWorkspace(t *testing.T, h http.Handler, c *http.Cookie, name string) store.Workspace {
	t.Helper()
	repo := addRepoWithCommit(t, h, c)
	rec := send(t, h, http.MethodPost, "/api/v1/workspaces", wsBody(repo.ID, name), c, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create workspace = %d: %s", rec.Code, rec.Body)
	}
	var ws store.Workspace
	if err := json.NewDecoder(rec.Body).Decode(&ws); err != nil {
		t.Fatal(err)
	}
	return ws
}

func wsPath(ws store.Workspace, action string) string {
	return fmt.Sprintf("/api/v1/workspaces/%d/%s", ws.ID, action)
}

func startBody(agent, prompt string) string {
	b, _ := json.Marshal(map[string]string{"agent": agent, "prompt": prompt})
	return string(b)
}

// waitState polls the workspace list until ws reaches state.
func waitState(t *testing.T, h http.Handler, c *http.Cookie, id int64, state string) store.Workspace {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var list []store.Workspace
		_ = json.NewDecoder(send(t, h, http.MethodGet, "/api/v1/workspaces", "", c, nil).Body).Decode(&list)
		for _, w := range list {
			if w.ID == id && w.State == state {
				return w
			}
		}
		time.Sleep(20 * time.Millisecond) // Polls a real process; there is no event to wait on here.
	}
	t.Fatalf("workspace %d never reached %s", id, state)
	return store.Workspace{}
}

func TestAgentsList(t *testing.T) {
	h := testHandler(t, testUI)
	c := signIn(t, h)
	rec := send(t, h, http.MethodGet, "/api/v1/agents", "", c, nil)
	var list []agentInfo
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "echo" || list[1].Name != "sleep" {
		t.Errorf("agents = %+v, want echo and sleep", list)
	}
}

func TestAgentRunAPI(t *testing.T) {
	h := testHandler(t, testUI)
	c := signIn(t, h)
	ws := newWorkspace(t, h, c, "echo")

	prompt := "fix it; rm -rf ~"
	rec := send(t, h, http.MethodPost, wsPath(ws, "start"), startBody("echo", prompt), c, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", rec.Code, rec.Body)
	}
	done := waitState(t, h, c, ws.ID, "exited")
	if done.ExitCode == nil || *done.ExitCode != 0 || done.Agent != "echo" {
		t.Errorf("ws = %+v, want exit 0 from echo", done)
	}

	rec = send(t, h, http.MethodGet, wsPath(ws, "output"), "", c, nil)
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "said: "+prompt) {
		t.Errorf("output = %q, want the prompt as one argument", rec.Body.String())
	}
}

func TestAgentStopAPI(t *testing.T) {
	h := testHandler(t, testUI)
	c := signIn(t, h)
	ws := newWorkspace(t, h, c, "sleeper")

	if rec := send(t, h, http.MethodPost, wsPath(ws, "stop"), "", c, nil); rec.Code != http.StatusConflict {
		t.Errorf("stop before start = %d, want 409", rec.Code)
	}
	if rec := send(t, h, http.MethodPost, wsPath(ws, "start"), startBody("sleep", ""), c, nil); rec.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", rec.Code, rec.Body)
	}
	if rec := send(t, h, http.MethodPost, wsPath(ws, "start"), startBody("sleep", ""), c, nil); rec.Code != http.StatusConflict {
		t.Errorf("start twice = %d, want 409", rec.Code)
	}
	rec := send(t, h, http.MethodPost, wsPath(ws, "stop"), "", c, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop = %d: %s", rec.Code, rec.Body)
	}
	var stopped store.Workspace
	_ = json.NewDecoder(rec.Body).Decode(&stopped)
	if stopped.State != "exited" {
		t.Errorf("state after stop = %s, want exited", stopped.State)
	}
}

func TestAgentStartRejects(t *testing.T) {
	h := testHandler(t, testUI)
	c := signIn(t, h)
	ws := newWorkspace(t, h, c, "bad")

	tests := []struct {
		name, body string
		want       int
	}{
		{"unknown agent", startBody("rm", "x"), http.StatusBadRequest},
		{"NUL in prompt", startBody("echo", "a\x00b"), http.StatusBadRequest},
		{"command in body", `{"agent":"echo","prompt":"x","command":["sh"]}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := send(t, h, http.MethodPost, wsPath(ws, "start"), tt.body, c, nil); rec.Code != tt.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
	if rec := send(t, h, http.MethodPost, "/api/v1/workspaces/999/start", startBody("echo", "x"), c, nil); rec.Code != http.StatusNotFound {
		t.Errorf("start in missing workspace = %d, want 404", rec.Code)
	}
}
