// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Workspaces } from "./Workspaces";
import type { Workspace, WorkspacesApi } from "./workspaces";

afterEach(cleanup);

const repos = [
  { id: 1, name: "app", path: "/src/app", addedAt: "" },
  { id: 2, name: "web", path: "/src/web", addedAt: "" },
];

function fakeApi(): WorkspacesApi & { created: [number, string][] } {
  let list: Workspace[] = [];
  const created: [number, string][] = [];
  return {
    created,
    list: async () => list,
    create: async (repoId, name) => {
      if (list.some((w) => w.slug === name)) throw new Error("a branch with this name already exists");
      created.push([repoId, name]);
      const ws = { id: list.length + 1, repoId, slug: name, branch: `tess/${name}`, base: "", path: "", state: "ready", createdAt: "" };
      list = [ws, ...list];
      return ws;
    },
    archive: async (id) => {
      list = list.map((w) => (w.id === id ? { ...w, state: "archived" } : w));
      return list.find((w) => w.id === id)!;
    },
  };
}

describe("Workspaces", () => {
  it("asks for a repo first", async () => {
    render(<Workspaces repos={[]} api={fakeApi()} />);
    expect(await screen.findByText("Add a repository to start a workspace.")).toBeTruthy();
  });

  it("creates in the chosen repo with a slug name, then archives", async () => {
    const api = fakeApi();
    render(<Workspaces repos={repos} api={api} />);

    fireEvent.change(await screen.findByLabelText("Repository"), { target: { value: "2" } });
    fireEvent.change(screen.getByLabelText("Workspace name"), { target: { value: "Fix Login" } });
    expect(screen.getByText("Branch: tess/fix-login")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Create workspace" }));

    expect(await screen.findByText("fix-login")).toBeTruthy();
    expect(api.created).toEqual([[2, "fix-login"]]);

    fireEvent.click(screen.getByRole("button", { name: "Archive fix-login" }));
    expect(await screen.findByText("archived")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Archive fix-login" })).toBeNull();
  });

  it("shows the daemon's error", async () => {
    render(<Workspaces repos={repos} api={fakeApi()} />);
    for (let i = 0; i < 2; i++) {
      fireEvent.change(await screen.findByLabelText("Workspace name"), { target: { value: "dup" } });
      fireEvent.click(screen.getByRole("button", { name: "Create workspace" }));
      await screen.findByText("dup");
    }
    expect((await screen.findByRole("alert")).textContent).toBe("a branch with this name already exists");
  });
});
