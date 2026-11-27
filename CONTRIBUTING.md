# Contributing to Tessaly

Thanks for helping. This guide covers how to set up, how to make a change, and the three rules every change follows: keep it secure, keep it simple, and keep it tested.

## Ground rules

1. Security first. Tessaly runs with your user's rights, so any bug that lets someone else reach the API puts the whole machine at risk. Read [docs/threat-model.md](docs/threat-model.md) before you touch `internal/server`.
2. Simple code wins. Prefer the standard library, plain functions and small interfaces. No new dependency without a short note in the PR on why the standard library is not enough. No frameworks for dependency injection, mocking or ORMs.
3. Tests with every change. A bug fix includes a test that fails before the fix. A feature includes unit tests for its logic.
4. Docs with every change. Document a feature in the same PR that adds it: `docs/`, `docs/wiki/` and the threat model describe only what exists in the code.

## Dev setup

Requirements:

- Go 1.27 or newer
- Node 20 or newer and pnpm 10
- git
- `golangci-lint` v2, for `make lint`
- Windows: develop inside WSL2 (Ubuntu or Debian). There is no native Windows build.

Steps:

```sh
git clone https://github.com/tessaly-dev/tessaly.git
cd tessaly
make setup      # installs UI dependencies from the lockfile
make test       # Go unit tests and UI unit tests
make check      # lint, vet, type checks, tests and a build
make dev        # builds the UI and runs the daemon on 127.0.0.1:7777
```

For UI work, run `make dev` in one terminal and `pnpm dev` in `web/` in another. Vite serves the UI on `127.0.0.1:5173` and forwards `/healthz` and `/api`, including the terminal WebSocket, to the daemon.

macOS and Linux are supported, on amd64 and arm64 (including Raspberry Pi). CI runs the Go tests on Linux amd64, Linux arm64 and macOS, so you only need one of them locally. On Windows, clone and build inside WSL2, not on the Windows file system.

Platform rules:

- Keep OS-specific code rare. When it is needed, use file suffixes (`_linux.go`, `_darwin.go`) behind one shared interface, not `runtime.GOOS` checks scattered through logic.
- Use `filepath`, never string joins with `/`.
- Never call `sh -c` or any shell with a command string. Use argv arrays.

## Project layout

| Path | What lives there |
| --- | --- |
| `cmd/tess` | CLI entry point (binary name `tess`) |
| `internal/agent` | Adapters that turn a prompt into an agent argv. They never run anything |
| `internal/auth` | Login token, passkeys (WebAuthn), sign-in sessions and the middleware that guards the API |
| `internal/config` | Finds the data folder (`~/.tessaly` or `$TESSALY_HOME`) and loads `config.toml` |
| `internal/gitx` | Runs git with fixed argv arrays, never a shell |
| `internal/ptyx` | Runs a program under a pseudo-terminal, keeps its recent output and stops its process group |
| `internal/server` | HTTP server, routes and security middleware |
| `internal/store` | SQLite database (`tessaly.db`) and its migrations in `internal/store/migrations` |
| `internal/workspace` | Workspace life cycle: branch, worktree and the state table in `state.go` |
| `internal/version` | Build version, set with `-ldflags` |
| `web/` | React UI, embedded into the binary with `go:embed` from `web/dist` |
| `scripts/` | `install.sh` for users, and its tests |
| `.goreleaser.yaml` | Release builds, see [Releasing](docs/wiki/Releasing.md) |
| `docs/` | Developer docs |
| `docs/wiki/` | Pages published to the GitHub wiki |

Add a row here when you add a package.

## Making a change

1. Open or find an issue first for anything larger than a small fix, so we agree on the approach before you write code.
2. Branch from `main`: `feat/<short-name>`, `fix/<short-name>` or `docs/<short-name>`.
3. Keep PRs small. One concern per PR. Under 400 changed lines is a good target.
4. Write commits in the Conventional Commits style: `feat(server): add request logging`.
5. Run `make check` before you push. CI runs the same checks, but only when Go, UI, build or workflow files change. Docs-only changes skip CI, and so do draft PRs until you mark them ready for review.
6. Fill in the PR template, including the security and docs items.

## Code style

Go:

- `gofmt` and `goimports` formatting, enforced in CI.
- Return errors with context: `fmt.Errorf("open config %s: %w", path, err)`.
- Accept interfaces, return structs. Define interfaces where they are used, and keep them small.
- Pass `context.Context` as the first argument to anything that does I/O or runs a process.
- No global state outside `main`.
- Start each new source file with `// SPDX-License-Identifier: Apache-2.0`.

TypeScript:

- Strict mode on. No `any` without a comment explaining why.
- Validate data from the API before you use it.

## Testing

| Kind | Command | When to add one |
| --- | --- | --- |
| Go unit | `make test-go` | Any logic: parsing, validation, routing, state changes |
| UI unit | `make test-ui` | Components with logic, hooks, API client |
| End to end | `make e2e` | Every UI feature: add a step to an existing flow in `web/e2e/` or a new spec |

Rules:

- Table-driven tests in Go.
- Write small fakes instead of using mocking libraries.
- No `time.Sleep` in tests. Wait on channels or poll with a deadline.
- Use `t.TempDir()` for any files. Never write outside it.
- Test the failure case, not only the happy path.
- A PR that adds or changes something in the UI adds or updates an end-to-end test, so the feature is checked in a real browser against the real binary.

### End-to-end tests

The specs in `web/e2e/` use Playwright and Chromium. Each spec file starts `bin/tess` with a fresh `TESSALY_HOME` in a temp folder, a `config.toml` with test agents, and a git repo with one commit (see `web/e2e/daemon.ts`). The tests then drive the UI like a person would. They fail on any console error, so a CSP violation or a script error breaks the build.

```sh
cd web && pnpm exec playwright install chromium   # once
make e2e                                          # builds bin/tess, then runs the specs
```

Test agents are plain commands such as `sh` and `cat`, so the tests need no real agent CLI and no network. Give each spec file its own port. CI runs the specs on Linux in the Build job.

## Security checklist for every PR

- [ ] No `sh -c` or other shell string execution. Commands use argv arrays.
- [ ] User input (paths, flags, request data) is validated before use.
- [ ] File paths are resolved and checked against their allowed root.
- [ ] New HTTP routes go through the existing middleware (Host check, cross-origin check and security headers) and reject methods they do not handle.
- [ ] New API routes sit behind sign-in (`auth.Require`). Only sign-in and sign-out are open.
- [ ] The server still binds to `127.0.0.1` only.
- [ ] No secrets or tokens written to logs.
- [ ] New files and directories use 0600 and 0700 permissions.
- [ ] New dependencies are pinned and justified in the PR description. New GitHub Actions are pinned to a full commit SHA.
- [ ] `docs/threat-model.md` lists any new control, with the test that proves it.

## Reporting security issues

Do not open a public issue. Follow [SECURITY.md](SECURITY.md) to report privately. We aim to reply within 3 working days.

## Documentation and the wiki

- Developer docs live in `docs/` and go through review like code.
- `docs/wiki/` holds the wiki pages. See [docs/wiki/README.md](docs/wiki/README.md) for how they stay in sync with the GitHub wiki.
- Add a page when you add a feature, not before.

## Code of conduct

This project follows the Contributor Covenant. See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## License

Tessaly is licensed under the Apache License 2.0 (see [LICENSE](LICENSE)). By contributing, you agree that your contributions are licensed under Apache-2.0, as section 5 of the license describes.

We use the Developer Certificate of Origin: sign off each commit with `git commit -s`. No CLA is required.

The sign-off adds a line like `Signed-off-by: Your Name <you@example.com>` to the commit message. It states that you wrote the change or have the right to submit it under the project license, as the [DCO](https://developercertificate.org/) describes. To fix a commit you forgot to sign: `git commit --amend -s` for the last commit, or `git rebase --signoff main` for a branch.
