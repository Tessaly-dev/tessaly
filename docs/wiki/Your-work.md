# Your work

The Home tab lists every workspace that is not archived by what it needs from you. The most urgent work sits at the top:

| Group | What is in it |
| --- | --- |
| Needs your input | Agents that run but have been quiet for 5 seconds, so they probably wait for an answer. Also runs that `tess serve` cut off when it stopped |
| Ready to review | Agents that exited with code 0, and agents you stopped |
| Failed | Agents that exited with another code, and workspaces whose setup failed |
| Running | Agents that are working and printing output |
| Not started | Workspaces where no agent has run yet |

Archived workspaces do not show. Empty groups are hidden. The list reloads every 2 seconds, so a workspace moves between groups on its own.

"Needs your input" is a guess. Tessaly cannot read what the agent prints, so an agent that thinks for 5 seconds without printing also lands there. See [Agents](Agents#waiting-for-you).

## Open a workspace

Click a workspace to open it beside the list. You see its repo, branch and state, and its agent panel: start or stop the agent, and type into its live terminal. Press Review changes to see its diff, see [Workspaces](Workspaces#review-changes). On a phone the panel shows below the list.

Press New workspace under the list to create one. See [Workspaces](Workspaces).

## Board

The Board tab shows every workspace as a card in a column for its stage: Not started, Running, Needs your input, Ready to review, Failed and Archived. Use it for an overview of all your tasks at once. Click a card to open the workspace on the Home tab. Archived cards show the branch that was kept, and do not open.

## Repositories

Add and remove repositories on the Repositories tab. See [Repositories](Repositories).
