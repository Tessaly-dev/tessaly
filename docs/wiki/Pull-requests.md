# Pull requests

When a workspace's work is ready, open a pull request from its branch on GitHub without leaving Tessaly.

## Before you start

Tessaly uses the [GitHub CLI](https://cli.github.com) and your own git setup. It never asks for, stores or sees a GitHub token.

1. Install `gh` on the machine that runs `tess serve`.
2. Run `gh auth login` there as the same user.
3. Make sure `git push` works from the repo without a password prompt: SSH with a key in your agent, or HTTPS with a credential helper. `gh auth setup-git` sets up the helper for you.
4. The repo needs a remote named `origin` on GitHub, and a git name and email (`git config user.name` and `user.email`).

## Open a pull request

Open the workspace on the Home tab. Under Pull request, check the title (it starts from the workspace name) and add a description, then press Open pull request. Tessaly:

1. commits any uncommitted changes and new files with the title as the message, using your git name and email
2. refuses if the branch has no commits since the workspace was created
3. pushes `tess/<name>` to `origin` and sets it as the upstream, never with `--force`
4. runs `gh pr create` to open a pull request into the repo's default branch, or uses the one already open for the branch

The panel then links the pull request. You cannot ship while an agent runs, so stop it first. A workspace cannot be archived while it ships.

## Push more changes

Once a pull request is open, the panel shows Push changes and a commit message field. Pressing it commits any changes and pushes them, and the pull request picks them up.

## Check status

Once a pull request is open, Tessaly asks gh for its CI checks (`gh pr checks`) every minute, and right after each push. The Pull request panel lists every check, failed ones first, with a link to each. The Home list shows the workspace under Ready to review as "Pull request open" with "checks passed", "checks running" or "checks failing".

The status lives in memory, so it shows again a minute after `tess serve` starts. If gh cannot answer, the panel keeps the last checks and says why. Archived workspaces are not checked.

## When something fails

The panel shows the message from git or gh, with any credentials in a remote URL hidden. Common causes:

| Message mentions | Fix |
| --- | --- |
| `gh` is not installed | Install the GitHub CLI and run `gh auth login` |
| `gh auth login` | Sign in to gh on the machine that runs `tess serve` |
| `could not read Username` or `Permission denied (publickey)` | Set up push credentials, see step 3 above. Git and gh run without a terminal, so they cannot ask for a password |
| `Please tell me who you are` | Set `git config user.name` and `user.email` |
| `rejected` | The remote branch has commits that your branch lacks. Tessaly never forces a push |

## API

| Method and path | Body | Reply |
| --- | --- | --- |
| `POST /api/v1/workspaces/{id}/ship` | `{"title": "Fix login", "body": "..."}` | `200` with the workspace and its `prUrl`. `400` for a bad title or a description over 64 KiB, `409` when an agent runs or there is nothing to ship, `502` with the git or gh message when a step fails |
