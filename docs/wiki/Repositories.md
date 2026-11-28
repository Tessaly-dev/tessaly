# Repositories

How repo records work. For how to add and remove repositories, see [Repositories](https://tessaly.dev/docs/repositories/) in the user docs.

Adding a repo only records its path. Tessaly does not change files in it.

## Adding

The path must be absolute and the top folder of a git work tree, not a subfolder and not a bare repo. Tessaly resolves symlinks and stores the real path. It names the repo after the folder, in lower case with only letters, numbers and dashes. If the name is taken it adds `-2`, `-3` and so on.

## Removing

Removing deletes Tessaly's record only. A foreign key from workspaces blocks it while the repo has any workspace, archived ones included, and the API replies `409`.

## API

All routes need a signed-in session. Send JSON with `Content-Type: application/json`.

| Method and path | Body | Reply |
| --- | --- | --- |
| `GET /api/v1/repos` | none | `200` with a list of `{id, name, path, addedAt}` |
| `POST /api/v1/repos` | `{"path": "/abs/path"}` | `201` with the repo, `400` if the path is not a repo top folder, `409` if it is already added |
| `DELETE /api/v1/repos/{id}` | none | `204`, `404` if there is no such repo, or `409` while it has workspaces |

## Where it is stored

Repos live in `~/.tessaly/tessaly.db`, a SQLite file with mode 0600.
