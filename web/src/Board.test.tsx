// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { Board } from "./Board";
import type { Workspace } from "./workspaces";

afterEach(cleanup);

function ws(over: Partial<Workspace>): Workspace {
  return { id: 1, repoId: 1, slug: "w", branch: "tess/w", base: "", path: "", state: "ready", createdAt: "", ...over };
}

const repos = [{ id: 1, name: "app", path: "/src/app", addedAt: "" }];

describe("Board", () => {
  it("puts each workspace in the column of its stage and opens it on click", () => {
    const onOpen = vi.fn();
    const list = [
      ws({ id: 1, slug: "asks", state: "running", waiting: true, agent: "aider" }),
      ws({ id: 2, slug: "old", state: "archived" }),
      ws({ id: 3, slug: "fresh" }),
    ];
    render(<Board list={list} repos={repos} onOpen={onOpen} />);

    const columns = screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent);
    expect(columns).toEqual([
      "Not started 1",
      "Running 0",
      "Needs your input 1",
      "Ready to review 0",
      "Failed 0",
      "Archived 1",
    ]);
    expect(within(screen.getByRole("region", { name: "Not started" })).getByText("fresh")).toBeTruthy();

    fireEvent.click(within(screen.getByRole("region", { name: "Needs your input" })).getByRole("button", { name: /asks/ }));
    expect(onOpen).toHaveBeenCalledWith(1);

    // Archived work has no agent to open, so its card is not a button.
    expect(within(screen.getByRole("region", { name: "Archived" })).queryByRole("button")).toBeNull();
  });
});
