# Threat model

This page lists the security controls the code enforces today. Add a row whenever a change adds a control, together with the test that proves it.

## Why this matters

Tessaly runs with your user's rights. Treat anyone who reaches the Tessaly API as able to act as you on your machine. Every route, file path and command is a security boundary.

## Controls

| Threat | Control | Test |
| --- | --- | --- |
| Another device on the network reaches the API | `tess serve` binds to `127.0.0.1` only. There is no flag to bind a wider address. | `TestAddr`, `TestRun` in `cmd/tess` |
| DNS rebinding: a web page points its own domain at `127.0.0.1` | Requests whose `Host` header is not `127.0.0.1:<port>` or `localhost:<port>` get `403` | `TestHostCheck` |
| Another OS user or local process calls the API | Every `/api/` route except sign-in and sign-out needs a session cookie. You get one by posting the login token, 32 random bytes kept in `~/.tessaly/token`. Tokens compare in constant time. | `TestAPINeedsSignIn`, `TestAPILoginFlow`, `TestLoginRejectsBadBodies` |
| Other users read the token | The data folder must be mode 0700 and the token file 0600, and a symlinked or malformed token file is refused. `tess serve` stops with an error otherwise. | `TestEnsureHome`, `TestLoadOrCreateTokenRejectsUnsafeFiles`, `TestServeRefusesOpenHome` |
| The token lands in log files | `tess serve` prints the sign-in link with the token only when stdout is a terminal. The token rides in the URL fragment, which browsers never send to the server, and the UI removes it from the address bar. Otherwise it prints only the token file path. | `TestServeStopsOnCancel`, `takeTokenFromHash` tests |
| Session theft through scripts | The session cookie is `HttpOnly` and `SameSite=Strict`, lasts 7 days and lives only in memory, so a daemon restart ends every session. Session IDs are 32 random bytes, stored only as SHA-256 hashes. | `TestAPILoginFlow`, `TestSessionExpires` |
| CSRF: a web page you visit posts to the local API | `http.CrossOriginProtection` rejects non-GET requests that the browser marks cross-site through `Sec-Fetch-Site`, or whose `Origin` does not match the host. `SameSite=Strict` keeps the cookie off cross-site requests too. | `TestAPIRejectsCrossSiteRequests` |
| A crafted repo path reaches git or the file system | `POST /api/v1/repos` takes only an absolute path. Tessaly resolves symlinks, checks it is a folder, and accepts it only when `git rev-parse --show-toplevel` from inside it returns the same folder. Subfolders, bare repos and flag-like values such as `--help` are refused. Git runs with a fixed argv, no shell, a 30 s timeout and `GIT_TERMINAL_PROMPT=0`. Removing a repo deletes only its record, never files. | `TestRepoRoot`, `TestRepoRootRejects`, `TestReposAPIRejectsBadInput` |
| A workspace name injects git flags, shell syntax or path segments | Names must match `^[a-z0-9][a-z0-9-]{0,47}$`, so they cannot start with `-` or hold `/`, `..` or spaces. Git calls put `--` before paths, and the worktree path is checked to stay inside `~/.tessaly/worktrees`. | `TestValidSlug`, `TestWorktreePathStaysInRoot`, `TestWorkspacesAPIRejectsBadInput` |
| Tessaly overwrites or deletes your work | Each workspace gets a new `tess/<name>` branch, and an existing branch is never reused. Archiving runs `git worktree remove` without `--force`, so a worktree with uncommitted or untracked files is refused, and only paths git lists as the repo's worktrees are removed. The branch stays. A repo with workspaces cannot be removed. | `TestCreateRefuses`, `TestArchiveRefusesDirtyWorktree`, `TestCreateMarksFailure`, `TestWorkspacesAPI` |
| Other users read agent work | Worktree folders are created with mode 0700 under the private data folder. | `TestCreateAndArchive` |
| The API runs a program of the caller's choice | The API only names an agent. The argv for each agent comes from `~/.tessaly/config.toml` on the host, and Tessaly refuses that file if other users can write it. Unknown request fields, such as `command`, are rejected. | `TestLoadRejects`, `TestAgentStartRejects` |
| Shell injection through the prompt | Agents run from an argv array with no shell. The prompt replaces only an argv element that is exactly `{prompt}`, so it stays one argument whatever it holds. Prompts over 32 KiB or with NUL bytes are refused. | `TestGenericCommand`, `TestGenericCommandRejectsBadPrompts`, `TestAgentRunAPI` |
| An agent reaches Tessaly's own settings or another repo | Agents start in their worktree with the daemon's environment minus `TESSALY_*` and `GIT_*` variables. | `TestStartRunsAgentInWorktree`, `TestAgentEnv` |
| Agents outlive the daemon or ignore Stop | Each agent leads its own process group. Stop sends SIGTERM to the group, then SIGKILL after 10 s. `tess serve` stops every agent before it exits, and marks agents left running by a crash as exited on the next start. | `TestStopEndsProgramAndChildren`, `TestStopAndStopAll`, `TestRecover` |
| A web page you visit opens the terminal socket | The socket needs a session, and `websocket.Accept` refuses an `Origin` that does not match the host. The `SameSite=Strict` cookie is not sent on cross-site requests either. Browser messages are capped at 64 KiB and resizes at 500 by 1000. | `TestTerminalRejects`, `TestTerminalStreamsAndTakesInput` |
| Agent output runs as script in the UI | Output is served as `text/plain` with `nosniff`. The terminal socket sends it as binary, which xterm.js draws as terminal cells, never as HTML, and the UI ignores text messages from the server. | `TestAgentRunAPI`, `terminalSession` tests |
| Other users read transcripts | Transcripts are kept in `~/.tessaly/logs/workspace-<id>.log` with mode 0600 in a 0700 folder. | `TestStartRunsAgentInWorktree` |
| Malformed or oversized API requests | JSON routes need `Content-Type: application/json`, cap bodies at 64 KiB and reject unknown fields. Errors reply with a short message, and internal errors are logged, not returned. | `TestReposAPIRejectsBadInput` |
| Other users read Tessaly's state | `tessaly.db` is created with mode 0600 inside the 0700 data folder. SQL uses bound parameters only. | `TestOpenCreatesPrivateFileAndMigrates` |
| Clickjacking or content injection in the UI | `Content-Security-Policy` that allows scripts only from Tessaly itself (`script-src 'self'`, no inline or eval). Inline styles are allowed because the terminal needs them, and they cannot run code. Also `frame-ancestors 'none'`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` on every response | `TestSecurityHeaders`, `TestCSPKeepsScriptsStrict` |
| Reading files outside the UI bundle | The UI is served from an embedded, read-only file system. The server indexes the bundle once at start and serves only files in that index, so folders are never listed and dotfiles and `..` paths return `404`. | `TestUI`, `TestUIPathTraversal` |
| Slow clients holding connections open | Read, write, header and idle timeouts, and a 64 KiB header limit | Covered by the `Serve` config |
| A tampered CI action or dependency | Actions pinned to full commit SHAs, `contents: read` permissions, `persist-credentials: false`, a frozen pnpm lockfile, install scripts off, `govulncheck` in CI, and Dependabot updates | CI |

## Known gaps

These are open risks in the code today. Each one needs a fix or a decision before v1.0.

| Gap | Why it matters |
| --- | --- |
| Cookies are not scoped by port | Browsers send the `tessaly_session` cookie to any server on `127.0.0.1`, whatever its port. A server on another local port that you open in the browser receives your session. `HttpOnly` stops page scripts from reading it, but not that server. |
| Agents run with your full user rights | There is no sandbox yet. An agent can read and change any file you can, not only its worktree. Use the agent's own permission settings. |
| Prompts show in the process list | The prompt is an argument, so other users on the machine can see it with `ps`. Do not put secrets in prompts. |
| Transcripts grow without limit | Transcript files are appended to and not rotated yet. |
| No cap on running agents | Each agent is a process with up to 1 MiB of output in memory. Many agents at once use memory and CPU without limit. |
| No rate limit on sign-in | The token has 256 bits of randomness, so guessing it is not practical. A limit would still cut noise in logs. |
