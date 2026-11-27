// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tessaly-dev/tessaly/internal/store"
)

func newWorkspace(t *testing.T, h http.Handler, c *creds, name string) store.Workspace {
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
func waitState(t *testing.T, h http.Handler, c *creds, id int64, state string) store.Workspace {
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
	if len(list) != 3 || list[0].Name != "echo" || list[1].Name != "file" || list[2].Name != "sleep" {
		t.Errorf("agents = %+v, want echo, file and sleep", list)
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

func TestAgentRunAPIWithPromptFile(t *testing.T) {
	h := testHandler(t, testUI)
	c := signIn(t, h)
	ws := newWorkspace(t, h, c, "file")

	prompt := "a task that must not show in ps"
	if rec := send(t, h, http.MethodPost, wsPath(ws, "start"), startBody("file", prompt), c, nil); rec.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", rec.Code, rec.Body)
	}
	waitState(t, h, c, ws.ID, "exited")
	rec := send(t, h, http.MethodGet, wsPath(ws, "output"), "", c, nil)
	if !strings.Contains(rec.Body.String(), prompt) {
		t.Errorf("output = %q, want the prompt read from the file", rec.Body.String())
	}
}

func TestWorkspaceListShowsWaiting(t *testing.T) {
	cfg := testConfig(t, testPort, testUI)
	cfg.Workspaces.IdleAfter = 0
	h := Handler(cfg)
	c := signIn(t, h)
	ws := newWorkspace(t, h, c, "waits")
	if rec := send(t, h, http.MethodPost, wsPath(ws, "start"), startBody("sleep", ""), c, nil); rec.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", rec.Code, rec.Body)
	}
	defer send(t, h, http.MethodPost, wsPath(ws, "stop"), "", c, nil)

	var list []workspaceView
	if err := json.NewDecoder(send(t, h, http.MethodGet, "/api/v1/workspaces", "", c, nil).Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].Waiting || list[0].State != "running" {
		t.Errorf("list = %+v, want one running workspace that waits", list)
	}
}

func TestAgentResumeAPI(t *testing.T) {
	cfg := testConfig(t, testPort, testUI)
	h := Handler(cfg)
	c := signIn(t, h)
	ws := newWorkspace(t, h, c, "cut-off")

	if rec := send(t, h, http.MethodPost, wsPath(ws, "start"), startBody("sleep", ""), c, nil); rec.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", rec.Code, rec.Body)
	}
	if rec := send(t, h, http.MethodPost, wsPath(ws, "resume"), "", c, nil); rec.Code != http.StatusConflict {
		t.Errorf("resume while running = %d, want 409", rec.Code)
	}
	// Shutting down cuts the run off, as when tess serve stops.
	cfg.Workspaces.StopAll(context.Background())
	cut := waitState(t, h, c, ws.ID, "exited")
	if cut.Ended != "interrupted" {
		t.Fatalf("ended = %q, want interrupted", cut.Ended)
	}

	rec := send(t, h, http.MethodPost, wsPath(ws, "resume"), "", c, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("resume = %d: %s", rec.Code, rec.Body)
	}
	done := waitState(t, h, c, ws.ID, "exited")
	if done.Ended != "exited" || done.Agent != "sleep" {
		t.Errorf("after resume = %+v, want exited on its own as sleep", done)
	}
	out := send(t, h, http.MethodGet, wsPath(ws, "output"), "", c, nil).Body.String()
	if !strings.Contains(out, "resumed") {
		t.Errorf("output = %q, want the resume command's output", out)
	}
	if rec := send(t, h, http.MethodPost, wsPath(ws, "resume"), "", c, nil); rec.Code != http.StatusConflict {
		t.Errorf("resume after a normal exit = %d, want 409", rec.Code)
	}
}

func TestAgentResumeNeedsResumeCommand(t *testing.T) {
	h := testHandler(t, testUI)
	c := signIn(t, h)
	ws := newWorkspace(t, h, c, "no-resume")
	if rec := send(t, h, http.MethodPost, wsPath(ws, "start"), startBody("echo", "hi"), c, nil); rec.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", rec.Code, rec.Body)
	}
	waitState(t, h, c, ws.ID, "exited")
	if rec := send(t, h, http.MethodPost, wsPath(ws, "resume"), "", c, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("resume without a resume command = %d, want 400", rec.Code)
	}
	if rec := send(t, h, http.MethodPost, "/api/v1/workspaces/999/resume", "", c, nil); rec.Code != http.StatusNotFound {
		t.Errorf("resume of a missing workspace = %d, want 404", rec.Code)
	}
}
