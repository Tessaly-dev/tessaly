# Workspaces

A workspace is one task in one repository. It has its own git branch and its own worktree, so several tasks can run side by side without touching each other's files.

## Create a workspace

On the Home tab, press New workspace (the form shows on its own when you have none), pick a repository, type a name, and press Create workspace. Tessaly then:

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

## Review changes

Open a workspace on the Home tab and press Review changes. Tessaly shows everything the workspace changed since the commit it was created from: commits on its branch, staged and unstaged edits, and new files that git does not ignore. Each file gets its own section with its added and removed lines. Press Refresh changes to load the diff again while an agent works.

The diff loads only when you ask, and stops at 1 MiB or 500 new files. The panel says so when it is cut. Tessaly runs `git diff` with `--no-ext-diff` and `--no-textconv`, so diff programs set in the repo's config never run.

To send the changes for review, open a pull request. See [Pull requests](Pull-requests).

## Archive a workspace

Press Archive. Tessaly runs `git worktree remove` without `--force`. If the worktree has uncommitted or untracked files, it refuses, so you do not lose work. Commit or delete those files first. The branch is kept.

A repository with workspaces cannot be removed from Tessaly.

## API

All routes need a signed-in session.

| Method and path | Body | Reply |
| --- | --- | --- |
| `GET /api/v1/workspaces` | none | `200` with the list, newest first. Each running workspace also has `waiting`, see [Agents](Agents#waiting-for-you) |
| `POST /api/v1/workspaces` | `{"repoId": 1, "name": "fix-login"}` | `201` with the workspace. `400` for a bad name or unknown repo, `409` if the branch or name exists |
| `GET /api/v1/workspaces/{id}/diff` | none | `200` with `{"patch": "<unified diff>", "truncated": false}`. `409` if the workspace has no worktree |
| `POST /api/v1/workspaces/{id}/archive` | none | `200` with the workspace. `409` if it has changes, an agent runs, or it is already archived |
