# Pull requests

How shipping a workspace and following its checks work. For setup, use and fixes for common errors, see [Pull requests](https://tessaly.dev/docs/pull-requests/) in the user docs.

Tessaly uses the user's own `gh` sign-in and git credentials, through `internal/forge` and `internal/gitx`. It never asks for, stores or sees a GitHub token. Git and gh run without a terminal, so they cannot prompt.

## Shipping

`POST /api/v1/workspaces/{id}/ship`:

1. commits any uncommitted changes and new files with the title as the message, using your git name and email
2. refuses if the branch has no commits since the workspace was created
3. pushes `tess/<name>` to `origin` and sets it as the upstream, never with `--force`
4. runs `gh pr create` to open a pull request into the repo's default branch, or uses the one already open for the branch

Shipping is refused while an agent runs, and archiving is refused while a workspace ships. Shipping again on a workspace with an open pull request commits and pushes the new changes, which is how Push changes in the UI works.

Tessaly's commits and pushes skip the repo's git hooks, such as `pre-commit` and `pre-push`. An agent can change those hooks, so running them would run its code as you. Every git call in `internal/gitx` sets `core.hooksPath=/dev/null` (`TestShipSkipsHooks`).

Errors come back as `502` with the git or gh message, with any credentials in a remote URL hidden.

## Check status

`internal/checks` asks gh for each open pull request's CI checks (`gh pr checks`) every minute, and right after each push. The status lives in memory, so it shows again a minute after `tess serve` starts. If gh cannot answer, the workspace keeps the last checks with the error. Archived workspaces are not checked.

## API

| Method and path | Body | Reply |
| --- | --- | --- |
| `POST /api/v1/workspaces/{id}/ship` | `{"title": "Fix login", "body": "..."}` | `200` with the workspace and its `prUrl`. `400` for a bad title or a description over 64 KiB, `409` when an agent runs or there is nothing to ship, `502` with the git or gh message when a step fails |
