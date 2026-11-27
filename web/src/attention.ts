// SPDX-License-Identifier: Apache-2.0
import type { Workspace } from "./workspaces";

// Attention says what a workspace needs from you, most urgent first.
export type Attention = "input" | "review" | "failed" | "running" | "idle" | "archived";

// The home list shows these groups, in this order. Archived work is left out.
export const homeGroups: { key: Attention; title: string }[] = [
  { key: "input", title: "Needs your input" },
  { key: "review", title: "Ready to review" },
  { key: "failed", title: "Failed" },
  { key: "running", title: "Running" },
  { key: "idle", title: "Not started" },
];

export function attention(ws: Workspace): Attention {
  switch (ws.state) {
    case "archived":
      return "archived";
    case "failed":
      return "failed";
    case "running":
      return ws.waiting ? "input" : "running";
    case "exited":
      if (ws.ended === "interrupted") return "input";
      if (ws.ended === "stopped" || ws.exitCode === 0) return "review";
      return "failed";
    default:
      // A workspace shipped by hand, with no agent run, waits for review too.
      return ws.prUrl ? "review" : "idle";
  }
}

// reason is one short line on why a workspace sits in its group.
export function reason(ws: Workspace): string {
  switch (ws.state) {
    case "creating":
      return "Setting up the worktree";
    case "ready":
      return ws.prUrl ? prReason(ws) : "No agent has run yet";
    case "running":
      return ws.waiting ? "Quiet for a while, probably waiting for you" : "Working";
    case "failed":
      return ws.error || "Failed";
    case "archived":
      return "Archived";
  }
  if (ws.ended === "interrupted") return "Cut off when tess stopped";
  if (ws.prUrl) return prReason(ws);
  if (ws.ended === "stopped") return "Stopped by you";
  if (ws.exitCode === 0) return "Finished";
  return `Exited with code ${ws.exitCode ?? "unknown"}`;
}

// prReason sums up a workspace whose pull request is open.
function prReason(ws: Workspace): string {
  switch (ws.checks?.state) {
    case "fail":
      return "Pull request open, checks failing";
    case "pending":
      return "Pull request open, checks running";
    case "pass":
      return "Pull request open, checks passed";
    default:
      return "Pull request open";
  }
}

// groupForHome sorts workspaces into the home groups, keeping the list's
// order inside each group, and drops empty groups.
export function groupForHome(list: Workspace[]): { key: Attention; title: string; items: Workspace[] }[] {
  return homeGroups
    .map((g) => ({ ...g, items: list.filter((ws) => attention(ws) === g.key) }))
    .filter((g) => g.items.length > 0);
}

// The board shows every stage as a column, in the order work moves through.
export const boardColumns: { key: Attention; title: string }[] = [
  { key: "idle", title: "Not started" },
  { key: "running", title: "Running" },
  { key: "input", title: "Needs your input" },
  { key: "review", title: "Ready to review" },
  { key: "failed", title: "Failed" },
  { key: "archived", title: "Archived" },
];

// groupForBoard sorts workspaces into every board column, empty ones included.
export function groupForBoard(list: Workspace[]): { key: Attention; title: string; items: Workspace[] }[] {
  return boardColumns.map((c) => ({ ...c, items: list.filter((ws) => attention(ws) === c.key) }));
}
