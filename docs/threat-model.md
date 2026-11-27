# Threat model

This page lists the security controls the code enforces today. Add a row whenever a change adds a control, together with the test that proves it.

## Why this matters

Tessaly runs with your user's rights. Treat anyone who reaches the Tessaly API as able to act as you on your machine. Every route, file path and command is a security boundary.

## Controls

| Threat | Control | Test |
| --- | --- | --- |
| Another device on the network reaches the API | `tess serve` binds to `127.0.0.1` only. There is no flag to bind a wider address. | `TestAddr`, `TestRun` in `cmd/tess` |
| DNS rebinding: a web page points its own domain at `127.0.0.1` | Requests whose `Host` header is not `127.0.0.1:<port>` or `localhost:<port>` get `403` | `TestHostCheck` |
| Clickjacking or content injection in the UI | `Content-Security-Policy` with `frame-ancestors 'none'`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` on every response | `TestSecurityHeaders` |
| Reading files outside the UI bundle | The UI is served from an embedded, read-only file system. Folders are never listed, dotfiles return `404`, and `..` paths are rejected. | `TestUI`, `TestUIPathTraversal` |
| Slow clients holding connections open | Read, write, header and idle timeouts, and a 64 KiB header limit | Covered by the `Serve` config |
| A tampered CI action or dependency | Actions pinned to full commit SHAs, `contents: read` permissions, `persist-credentials: false`, a frozen pnpm lockfile, install scripts off, `govulncheck` in CI, and Dependabot updates | CI |
