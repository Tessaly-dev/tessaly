# Workspaces

How workspaces work inside Tessaly. For how to create, review and archive them, see [Workspaces](https://tessaly.dev/docs/workspaces/) in the user docs.

A workspace is one task in one repository, with its own git branch and worktree.

## Creating

Creating a workspace:

1. turns the name into lower-case letters, digits and dashes, up to 48 characters, such as `fix-login`
2. refuses if the branch `tess/<name>` already exists, because it never reuses a branch
3. creates `tess/<name>` from the repo's current `HEAD`
4. checks it out in a new worktree at `~/.tessaly/worktrees/<repo>/<name>`

The repo needs at least one commit.

## States

| State | Meaning |
| --- | --- |
| `creating` | The branch and worktree are being made |
| `ready` | The worktree exists and no agent has run yet |
| `running` | An agent runs in the worktree. See [Agents](Agents) |
| `exited` | The agent has exited. Its exit code shows next to the workspace |
| `failed` | A step failed. The error shows next to the workspace |
| `archived` | The worktree is gone. The branch stays |

Only these moves are allowed: `creating` to `ready` or `failed`, `ready` to `running`, `archived` or `failed`, `running` to `exited`, `exited` to `running` or `archived`, and `failed` to `archived`. The table lives in `internal/workspace/state.go`.

## The diff

The diff covers everything since the commit the workspace was created from: commits on its branch, staged and unstaged edits, and new files that git does not ignore. It loads only when the user asks, and stops at 1 MiB or 500 new files, with `truncated: true`. Tessaly runs `git diff` with `--no-ext-diff` and `--no-textconv`, so diff programs set in the repo's config never run.

## Archiving

Archiving runs `git worktree remove` without `--force`, so it refuses while the worktree has uncommitted or untracked files. The branch is kept. A repository with workspaces cannot be removed from Tessaly.

## API

All routes need a signed-in session.

| Method and path | Body | Reply |
| --- | --- | --- |
| `GET /api/v1/workspaces` | none | `200` with the list, newest first. Each running workspace also has `waiting`, see [Agents](Agents#waiting-for-you) |
| `POST /api/v1/workspaces` | `{"repoId": 1, "name": "fix-login"}` | `201` with the workspace. `400` for a bad name or unknown repo, `409` if the branch or name exists |
| `GET /api/v1/workspaces/{id}/diff` | none | `200` with `{"patch": "<unified diff>", "truncated": false}`. `409` if the workspace has no worktree |
| `POST /api/v1/workspaces/{id}/archive` | none | `200` with the workspace. `409` if it has changes, an agent runs, or it is already archived |
