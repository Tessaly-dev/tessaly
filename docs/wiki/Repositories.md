# Repositories

Tessaly works on git repositories you add. Adding a repo only records its path. Tessaly does not change files in it.

## Add a repository

Sign in, type the absolute path of the repo's top folder, and press Add repository. The path must be:

- absolute, such as `/home/you/code/app`
- the top folder of a git work tree, not a subfolder and not a bare repo

Tessaly resolves symlinks and stores the real path. It names the repo after the folder, in lower case with only letters, numbers and dashes. If the name is taken it adds `-2`, `-3` and so on.

## Remove a repository

Press Remove next to it. This deletes Tessaly's record only. Your files and git history stay as they are.

## API

All routes need a signed-in session. Send JSON with `Content-Type: application/json`.

| Method and path | Body | Reply |
| --- | --- | --- |
| `GET /api/v1/repos` | none | `200` with a list of `{id, name, path, addedAt}` |
| `POST /api/v1/repos` | `{"path": "/abs/path"}` | `201` with the repo, `400` if the path is not a repo top folder, `409` if it is already added |
| `DELETE /api/v1/repos/{id}` | none | `204`, `404` if there is no such repo, or `409` while it has workspaces |

## Where it is stored

Repos live in `~/.tessaly/tessaly.db`, a SQLite file with mode 0600.
