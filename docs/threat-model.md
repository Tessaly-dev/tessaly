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
| Clickjacking or content injection in the UI | `Content-Security-Policy` with `frame-ancestors 'none'`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` on every response | `TestSecurityHeaders` |
| Reading files outside the UI bundle | The UI is served from an embedded, read-only file system. The server indexes the bundle once at start and serves only files in that index, so folders are never listed and dotfiles and `..` paths return `404`. | `TestUI`, `TestUIPathTraversal` |
| Slow clients holding connections open | Read, write, header and idle timeouts, and a 64 KiB header limit | Covered by the `Serve` config |
| A tampered CI action or dependency | Actions pinned to full commit SHAs, `contents: read` permissions, `persist-credentials: false`, a frozen pnpm lockfile, install scripts off, `govulncheck` in CI, and Dependabot updates | CI |

## Known gaps

These are open risks in the code today. Each one needs a fix or a decision before v1.0.

| Gap | Why it matters |
| --- | --- |
| Cookies are not scoped by port | Browsers send the `tessaly_session` cookie to any server on `127.0.0.1`, whatever its port. A server on another local port that you open in the browser receives your session. `HttpOnly` stops page scripts from reading it, but not that server. |
| No rate limit on sign-in | The token has 256 bits of randomness, so guessing it is not practical. A limit would still cut noise in logs. |
