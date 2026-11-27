// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/coder/websocket"

	"github.com/tessaly-dev/tessaly/internal/ptyx"
)

// maxTerminalMessage caps one message from the browser: a keystroke batch, a
// paste or a resize.
const maxTerminalMessage = 64 << 10

// writeTimeout drops a browser that stops reading.
const writeTimeout = 10 * time.Second

// terminalMessage is what the browser sends over the socket.
type terminalMessage struct {
	Type string `json:"type"` // "input" or "resize"
	Data string `json:"data,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// terminal streams a workspace's agent terminal over a WebSocket. The server
// sends output as binary messages: first the recent output, then new output
// as it comes. The browser sends JSON text messages with input or a new size.
// When no agent runs, it sends the end of the transcript and closes.
func (h agentHandlers) terminal(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws.find(w, r)
	if !ok {
		return
	}
	// net/http clears the server's read and write deadlines when Accept
	// hijacks the connection, so a long stream is not cut. writeTimeout
	// still guards each write.
	// Accept checks that Origin matches Host, so other sites cannot open
	// the socket even though WebSockets skip CORS.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept has written the error reply.
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(maxTerminalMessage)
	ctx := r.Context()

	proc := h.ws.manager.Process(ws.ID)
	if proc == nil {
		out, err := tail(h.ws.manager.TranscriptPath(ws.ID), ptyx.BufferSize)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = conn.Close(websocket.StatusInternalError, "cannot read transcript")
			return
		}
		if write(ctx, conn, out) == nil {
			_ = conn.Close(websocket.StatusNormalClosure, "not running")
		}
		return
	}

	snapshot, updates, cancel := proc.Subscribe()
	defer cancel()

	// Read input in the background. It ends when the browser goes away.
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		defer stop()
		readInput(ctx, conn, proc)
	}()

	if write(ctx, conn, snapshot) != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case b, ok := <-updates:
			if !ok {
				// The agent exited, or this browser fell behind. Either way
				// the browser reconnects to see where things stand.
				_ = conn.Close(websocket.StatusNormalClosure, "stream ended")
				return
			}
			if write(ctx, conn, b) != nil {
				return
			}
		}
	}
}

// readInput passes keystrokes and resizes from the browser to the agent.
func readInput(ctx context.Context, conn *websocket.Conn, proc *ptyx.Process) {
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		var msg terminalMessage
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.Type {
		case "input":
			_, _ = proc.Write([]byte(msg.Data))
		case "resize":
			if msg.Rows > 0 && msg.Cols > 0 && msg.Rows <= 500 && msg.Cols <= 1000 {
				_ = proc.Resize(msg.Rows, msg.Cols)
			}
		}
	}
}

func write(ctx context.Context, conn *websocket.Conn, b []byte) error {
	if len(b) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(ctx, websocket.MessageBinary, b)
}
