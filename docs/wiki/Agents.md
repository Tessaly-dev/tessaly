# Agents

How Tessaly runs agents. For how to set up and run an agent, see [Agents](https://tessaly.dev/docs/agents/) in the user docs.

Tessaly runs agent CLIs in workspaces. Today it has one adapter, the generic adapter: it runs any command you list in the config file, under a terminal, in the workspace's worktree. It does not read the agent's output, so it knows only whether the agent runs, whether it has been quiet for a while, and how it ended.

## Config

Agents come from `~/.tessaly/config.toml`, read when `tess serve` starts (`internal/config`):

```toml
[agents.aider]
command = ["aider", "--message-file", "{prompt_file}"]

[agents.claude]
command = ["claude", "{prompt}"]
resume = ["claude", "--continue"]

[agents.shell]
command = ["bash"]
```

- `command` is an argv array. Tessaly never runs it through a shell, so `|`, `&&` and `$VAR` have no special meaning.
- An element that is exactly `{prompt}` is replaced by the task prompt, as one argument. `{prompt}` inside a longer element is left alone.
- An element that is exactly `{prompt_file}` is replaced by the path of a file that holds the prompt. Use it when the CLI can read its task from a file. Arguments show in `ps` to every user on the machine, and a file does not. Tessaly writes the file to `~/.tessaly/prompts/workspace-<id>.txt` with mode 0600 just before the agent starts, and deletes it when the agent exits.
- `resume` is optional. It is the argv that picks the agent's last session back up in the worktree. It cannot use `{prompt}` or `{prompt_file}`, because Tessaly does not keep the prompt. See [Resume after a restart](#resume-after-a-restart).
- Agent names use lower-case letters, digits and dashes, up to 32 characters.
- Tessaly refuses to start if other users can write the file (`chmod go-w ~/.tessaly/config.toml`).

## Running

A started agent:

- starts in the worktree, under a terminal 120 columns wide
- gets your environment minus `TESSALY_*` and `GIT_*` variables, plus `TERM=xterm-256color`, `TESSALY_WORKSPACE` and `TESSALY_BRANCH`
- writes its output to `~/.tessaly/logs/workspace-<id>.log` (mode 0600). Tessaly keeps the last 1 MiB in memory

A transcript stops growing at 64 MiB and ends with a note that later output is not saved. The live terminal keeps working. The next run in that workspace moves the full transcript to `workspace-<id>.log.1`, replacing any older one, and starts a new file.

Up to 16 agents run at once. Stop one to start another.

The UI shows the terminal live over the [terminal socket](#terminal-socket), and sends keystrokes and resizes back.

Stopping sends SIGTERM to the agent and every process it started, and SIGKILL after 10 seconds. When the agent exits, the workspace moves to `exited` with the exit code. An exited workspace can start an agent again.

Stopping `tess serve` stops every agent. A workspace that was running when Tessaly crashed shows `exited` on the next start.

### Resume after a restart

When `tess serve` stops or crashes while an agent runs, the run is cut off. On the next start the workspace has `ended: "interrupted"` and sits under Needs your input on the Home tab. Nothing restarts on its own.

`POST /api/v1/workspaces/{id}/resume` runs the agent's `resume` command in the worktree, as the same agent.

### How a run ended

Each workspace keeps how its last run ended, in the `ended` field:

| `ended` | Meaning |
| --- | --- |
| `exited` | The agent exited on its own. `exitCode` says how |
| `stopped` | The user stopped it |
| `interrupted` | `tess serve` stopped or crashed while the agent ran |

The field is empty while an agent runs and before the first run.

### Waiting for you

A running agent that has printed nothing for 5 seconds shows `"waiting": true` in the workspace list. Agents print while they work, so a quiet terminal usually means a question or a prompt for you. This is a guess, because the generic adapter cannot read the output: an agent that works without printing for 5 seconds also counts as waiting.

## API

All routes need a signed-in session.

| Method and path | Body | Reply |
| --- | --- | --- |
| `GET /api/v1/agents` | none | `200` with `[{name, command, resume}]`. `resume` is left out when the agent has none |
| `POST /api/v1/workspaces/{id}/start` | `{"agent": "aider", "prompt": "..."}` | `200` with the workspace. `400` for an unknown agent, a bad prompt or a missing program. `409` unless the workspace is ready or exited. `429` when 16 agents run already |
| `POST /api/v1/workspaces/{id}/resume` | none | `200` with the workspace. `400` if the last run's agent has no `resume` command. `409` unless tess cut off the last run. `429` when 16 agents run already |
| `POST /api/v1/workspaces/{id}/stop` | none | `200` once the agent has exited. `409` if none runs |
| `GET /api/v1/workspaces/{id}/output` | none | `200` with the last 1 MiB of output as `text/plain` |
| `GET /api/v1/workspaces/{id}/terminal` | WebSocket | See below |

### Terminal socket

`/api/v1/workspaces/{id}/terminal` upgrades to a WebSocket. The browser must send the session cookie, offer the subprotocols `tessaly` and `tessaly.key.<key>` with the session key from [sign-in](Sign-in), and send an `Origin` that matches the host. The server picks `tessaly`.

- The server sends binary messages with raw terminal output: first the last 1 MiB, then new output as it comes. When no agent runs, it sends the end of the transcript and closes.
- The browser sends JSON text messages: `{"type": "input", "data": "ls\r"}` for keystrokes, and `{"type": "resize", "rows": 40, "cols": 120}` for a new size. Each message can be up to 64 KiB.
- The server closes the socket when the agent exits, or when the browser falls more than 256 messages behind. Reconnect to see the current state.
