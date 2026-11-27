# Dev setup

You need Go 1.24 or newer, Node 20 or newer, pnpm 10, git, and `golangci-lint` v2 for linting. macOS and Linux (amd64 or arm64) work. On Windows, work inside WSL2.

```sh
git clone https://github.com/tessaly-dev/tessaly.git
cd tessaly
make setup   # UI dependencies from the lockfile
make test    # Go and UI unit tests
make check   # lint, type checks, tests and a build, as CI runs them
make dev     # build and run on http://127.0.0.1:7777
```

For UI work, keep `make dev` running and start `pnpm dev` in `web/`. Vite serves the UI on `127.0.0.1:5173` and forwards `/healthz` to the daemon.

Every commit needs a DCO sign-off: `git commit -s`. Read [CONTRIBUTING.md](https://github.com/tessaly-dev/tessaly/blob/main/CONTRIBUTING.md) for the full rules and the security checklist.
