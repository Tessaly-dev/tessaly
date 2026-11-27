# Agents

Tessaly runs agent CLIs in workspaces. Today it has one adapter, the generic adapter: it runs any command you list in the config file, under a terminal, in the workspace's worktree. It does not read the agent's output, so status is only running or exited with a code.

## Set up an agent

Add agents to `~/.tessaly/config.toml`, then restart `tess serve`:

```toml
[agents.aider]
command = ["aider", "--message", "{prompt}"]

[agents.claude]
command = ["claude", "{prompt}"]

[agents.shell]
command = ["bash"]
```

- `command` is an argv array. Tessaly never runs it through a shell, so `|`, `&&` and `$VAR` have no special meaning.
- An element that is exactly `{prompt}` is replaced by the task prompt, as one argument. `{prompt}` inside a longer element is left alone.
- Agent names use lower-case letters, digits and dashes, up to 32 characters.
- Tessaly refuses to start if other users can write the file (`chmod go-w ~/.tessaly/config.toml`).

## Run an agent

Open a workspace's Agent panel, pick an agent, type a prompt and press Start agent. The agent:

- starts in the worktree, under a terminal 120 columns wide
- gets your environment minus `TESSALY_*` and `GIT_*` variables, plus `TERM=xterm-256color`, `TESSALY_WORKSPACE` and `TESSALY_BRANCH`
- writes its output to `~/.tessaly/logs/workspace-<id>.log` (mode 0600). Tessaly keeps the last 1 MiB in memory

The panel shows the agent's terminal live. Click into it to type: keystrokes go to the agent as if you typed in a local terminal. Resizing the window resizes the agent's terminal. After the agent exits, the panel shows the end of the transcript.

Press Stop agent to end it. Tessaly sends SIGTERM to the agent and every process it started, and SIGKILL after 10 seconds. When the agent exits, the workspace shows `exited` and the exit code. You can start an agent again in an exited workspace.

Stopping `tess serve` stops every agent. A workspace that was running when Tessaly crashed shows `exited` on the next start.

## API

All routes need a signed-in session.

| Method and path | Body | Reply |
| --- | --- | --- |
| `GET /api/v1/agents` | none | `200` with `[{name, command}]` |
| `POST /api/v1/workspaces/{id}/start` | `{"agent": "aider", "prompt": "..."}` | `200` with the workspace. `400` for an unknown agent, a bad prompt or a missing program. `409` unless the workspace is ready or exited |
| `POST /api/v1/workspaces/{id}/stop` | none | `200` once the agent has exited. `409` if none runs |
| `GET /api/v1/workspaces/{id}/output` | none | `200` with the last 1 MiB of output as `text/plain` |
| `GET /api/v1/workspaces/{id}/terminal` | WebSocket | See below |

### Terminal socket

`/api/v1/workspaces/{id}/terminal` upgrades to a WebSocket. The browser must send the session cookie and an `Origin` that matches the host.

- The server sends binary messages with raw terminal output: first the last 1 MiB, then new output as it comes. When no agent runs, it sends the end of the transcript and closes.
- The browser sends JSON text messages: `{"type": "input", "data": "ls\r"}` for keystrokes, and `{"type": "resize", "rows": 40, "cols": 120}` for a new size. Each message can be up to 64 KiB.
- The server closes the socket when the agent exits, or when the browser falls more than 256 messages behind. Reconnect to see the current state.
