// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import { attention, groupForBoard, groupForHome, reason } from "./attention";
import type { Workspace } from "./workspaces";

function ws(over: Partial<Workspace>): Workspace {
  return { id: 1, repoId: 1, slug: "w", branch: "tess/w", base: "", path: "", state: "ready", createdAt: "", ...over };
}

describe("attention", () => {
  it.each([
    [{ state: "running", waiting: true }, "input", "Quiet for a while, probably waiting for you"],
    [{ state: "exited", ended: "interrupted", exitCode: -1 }, "input", "Cut off when tess stopped"],
    [{ state: "exited", ended: "exited", exitCode: 0 }, "review", "Finished"],
    [{ state: "exited", ended: "stopped", exitCode: -1 }, "review", "Stopped by you"],
    [{ state: "exited", ended: "exited", exitCode: 2 }, "failed", "Exited with code 2"],
    [{ state: "failed", error: "git said no" }, "failed", "git said no"],
    [{ state: "running" }, "running", "Working"],
    [{ state: "ready" }, "idle", "No agent has run yet"],
    [{ state: "creating" }, "idle", "Setting up the worktree"],
    [{ state: "archived" }, "archived", "Archived"],
  ] as [Partial<Workspace>, string, string][])("%o is %s", (over, want, why) => {
    expect(attention(ws(over))).toBe(want);
    expect(reason(ws(over))).toBe(why);
  });

  it("treats a run from before tess kept how runs end by its exit code", () => {
    expect(attention(ws({ state: "exited", exitCode: 0 }))).toBe("review");
    expect(attention(ws({ state: "exited", exitCode: 1 }))).toBe("failed");
  });
});

describe("groupForHome", () => {
  it("orders groups by urgency, keeps list order, and leaves out archived and empty groups", () => {
    const list = [
      ws({ id: 1, state: "running" }),
      ws({ id: 2, state: "exited", exitCode: 0 }),
      ws({ id: 3, state: "running", waiting: true }),
      ws({ id: 4, state: "archived" }),
      ws({ id: 5, state: "exited", exitCode: 0 }),
    ];
    expect(groupForHome(list).map((g) => [g.key, g.items.map((w) => w.id)])).toEqual([
      ["input", [3]],
      ["review", [2, 5]],
      ["running", [1]],
    ]);
  });
});

describe("groupForBoard", () => {
  it("keeps every column, archived and empty ones too, in stage order", () => {
    const list = [ws({ id: 1, state: "archived" }), ws({ id: 2, state: "running" })];
    expect(groupForBoard(list).map((c) => [c.key, c.items.map((w) => w.id)])).toEqual([
      ["idle", []],
      ["running", [2]],
      ["input", []],
      ["review", []],
      ["failed", []],
      ["archived", [1]],
    ]);
  });
});
