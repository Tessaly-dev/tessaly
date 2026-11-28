# Your work

How the Home list and the Board group workspaces. For how to use them, see [Home and Board](https://tessaly.dev/docs/home-and-board/) in the user docs.

The UI works out each workspace's group in `web/src/attention.ts`, from the workspace list the API returns. The Home tab lists groups in this order, most urgent first:

| Group | Rule |
| --- | --- |
| Needs your input | `running` with `waiting: true` (see [Agents](Agents#waiting-for-you)), or `exited` with `ended: "interrupted"`. An interrupted run shows a Resume button when its agent has a `resume` command |
| Ready to review | `exited` with `ended: "stopped"` or exit code 0, or `ready` with an open pull request. The line under it shows the CI check status, see [Pull requests](Pull-requests#check-status) |
| Failed | `failed`, or `exited` with another code |
| Running | `running` and not waiting |
| Not started | `creating`, or `ready` with no pull request |

Archived workspaces do not show on Home, and empty groups are hidden. The UI polls `GET /api/v1/workspaces` every 2 seconds.

"Needs your input" is a guess, because the generic adapter cannot read what the agent prints.

## Board

The Board tab shows every workspace as a card in a column for its group, plus an Archived column. Clicking a card opens the workspace on the Home tab. Archived cards do not open.
