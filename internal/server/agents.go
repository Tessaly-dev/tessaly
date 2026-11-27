// SPDX-License-Identifier: Apache-2.0

package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"sort"

	"github.com/tessaly-dev/tessaly/internal/agent"
	"github.com/tessaly-dev/tessaly/internal/config"
	"github.com/tessaly-dev/tessaly/internal/ptyx"
	"github.com/tessaly-dev/tessaly/internal/workspace"
)

type agentInfo struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
}

type agentHandlers struct {
	agents map[string]config.Agent
	ws     workspaceHandlers
}

func (h agentHandlers) list(w http.ResponseWriter, _ *http.Request) {
	list := make([]agentInfo, 0, len(h.agents))
	for name, a := range h.agents {
		list = append(list, agentInfo{Name: name, Command: a.Command})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	writeJSON(w, http.StatusOK, list)
}

// start runs a configured agent in a workspace. The request names the agent;
// the command itself only ever comes from the config file on this machine.
func (h agentHandlers) start(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws.find(w, r)
	if !ok {
		return
	}
	var body struct {
		Agent  string `json:"agent"`
		Prompt string `json:"prompt"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	cfg, ok := h.agents[body.Agent]
	if !ok {
		writeError(w, http.StatusBadRequest, "no such agent. Add it to config.toml")
		return
	}
	argv, err := agent.Generic{Argv: cfg.Command}.Command(body.Prompt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ws, err = h.ws.manager.Start(r.Context(), ws, body.Agent, argv)
	switch {
	case errors.Is(err, workspace.ErrConflict):
		writeError(w, http.StatusConflict, "an agent can start only in a ready or exited workspace")
	case errors.Is(err, workspace.ErrTooManyRunning):
		writeError(w, http.StatusTooManyRequests, err.Error())
	case err != nil:
		// A missing program is the usual cause, and the message names only
		// the program from the user's own config.
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSON(w, http.StatusOK, ws)
	}
}

func (h agentHandlers) stop(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws.find(w, r)
	if !ok {
		return
	}
	err := h.ws.manager.Stop(r.Context(), ws.ID)
	if errors.Is(err, workspace.ErrNotRunning) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	ws, ok = h.ws.find(w, r)
	if ok {
		writeJSON(w, http.StatusOK, ws)
	}
}

// output replies with the agent's recent terminal output: from memory while
// it runs, else the end of the transcript file.
func (h agentHandlers) output(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws.find(w, r)
	if !ok {
		return
	}
	var out []byte
	if proc := h.ws.manager.Process(ws.ID); proc != nil {
		out = proc.Output()
	} else {
		var err error
		out, err = tail(h.ws.manager.TranscriptPath(ws.ID), ptyx.BufferSize)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			serverError(w, err)
			return
		}
	}
	// Plain text with nosniff, so a browser never renders agent output as HTML.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
}

// tail returns up to the last n bytes of the file at path.
func tail(path string, n int64) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- path is built from the workspace id.
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if off := info.Size() - n; off > 0 {
		if _, err := f.Seek(off, io.SeekStart); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(f, n))
}
