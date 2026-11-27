// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { Home } from "./Home";
import { useWorkspaces } from "./useWorkspaces";
import type { Workspace, WorkspacesApi } from "./workspaces";

// xterm.js needs a real browser. Its socket logic is tested in terminalSession.test.ts.
vi.mock("./Terminal", () => ({ Terminal: () => null }));

afterEach(cleanup);

const repos = [
  { id: 1, name: "app", path: "/src/app", addedAt: "" },
  { id: 2, name: "web", path: "/src/web", addedAt: "" },
];

function ws(over: Partial<Workspace>): Workspace {
  return { id: 1, repoId: 1, slug: "w", branch: "tess/w", base: "", path: "", state: "ready", createdAt: "", ...over };
}

function fakeApi(start: Workspace[] = []): WorkspacesApi & { created: [number, string][] } {
  let list = start;
  const created: [number, string][] = [];
  const update = (id: number, over: Partial<Workspace>) => {
    list = list.map((w) => (w.id === id ? { ...w, ...over } : w));
    return list.find((w) => w.id === id)!;
  };
  return {
    created,
    list: async () => list,
    create: async (repoId, name) => {
      if (list.some((w) => w.slug === name)) throw new Error("a branch with this name already exists");
      created.push([repoId, name]);
      const w = ws({ id: list.length + 1, repoId, slug: name, branch: `tess/${name}` });
      list = [w, ...list];
      return w;
    },
    archive: async (id) => update(id, { state: "archived" }),
    agents: async () => [
      { name: "aider", command: ["aider", "{prompt}"], resume: ["aider", "--restore-chat-history"] },
      { name: "plain", command: ["plain"] },
    ],
    start: async (id) => update(id, { state: "running", agent: "aider" }),
    stop: async (id) => update(id, { state: "exited", exitCode: -1, ended: "stopped" }),
    resume: async (id) => update(id, { state: "running", ended: undefined }),
  };
}

// Harness wires Home to the real list hook, as App does.
function Harness({ api }: { api: WorkspacesApi }) {
  const [selected, setSelected] = useState<number | null>(null);
  const work = useWorkspaces(api);
  return (
    <Home
      list={work.list}
      repos={repos}
      agents={work.agents}
      api={api}
      refresh={work.refresh}
      selected={selected}
      onSelect={setSelected}
    />
  );
}

describe("Home", () => {
  it("groups work by what needs you, most urgent first", async () => {
    const api = fakeApi([
      ws({ id: 1, slug: "busy", state: "running", agent: "aider" }),
      ws({ id: 2, slug: "done", state: "exited", exitCode: 0, ended: "exited" }),
      ws({ id: 3, slug: "asks", state: "running", waiting: true, repoId: 2 }),
      ws({ id: 4, slug: "broke", state: "exited", exitCode: 1, ended: "exited" }),
      ws({ id: 5, slug: "old", state: "archived" }),
    ]);
    render(<Harness api={api} />);

    await screen.findByText("asks");
    const titles = screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent);
    expect(titles).toEqual(["Needs your input 1", "Ready to review 1", "Failed 1", "Running 1"]);
    const input = screen.getByRole("region", { name: "Needs your input" });
    expect(within(input).getByText("web")).toBeTruthy();
    expect(within(input).getByText("Quiet for a while, probably waiting for you")).toBeTruthy();
    expect(screen.getByText("Exited with code 1")).toBeTruthy();
    expect(screen.queryByText("old")).toBeNull();
  });

  it("opens a workspace beside the list and runs its agent", async () => {
    render(<Harness api={fakeApi([ws({ id: 1, slug: "task" })])} />);
    expect(await screen.findByText("Pick a workspace to see its agent.")).toBeTruthy();

    fireEvent.click(await screen.findByRole("button", { name: /task/ }));
    const detail = screen.getByRole("region", { name: "Workspace" });
    expect(within(detail).getByRole("heading", { name: "task" })).toBeTruthy();

    fireEvent.click(within(detail).getByRole("button", { name: "Start agent" }));
    expect(await screen.findByRole("region", { name: "Running" })).toBeTruthy();
    expect(within(detail).queryByRole("button", { name: "Archive task" })).toBeNull();

    fireEvent.click(within(detail).getByRole("button", { name: "Stop agent" }));
    const review = await screen.findByRole("region", { name: "Ready to review" });
    expect(within(review).getByText("Stopped by you")).toBeTruthy();
  });

  it("creates a workspace in the chosen repo and opens it", async () => {
    const api = fakeApi();
    render(<Harness api={api} />);

    fireEvent.change(await screen.findByLabelText("Repository"), { target: { value: "2" } });
    fireEvent.change(screen.getByLabelText("Workspace name"), { target: { value: "Fix Login" } });
    expect(screen.getByText("Branch: tess/fix-login")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Create workspace" }));

    expect(await screen.findByRole("heading", { name: "fix-login" })).toBeTruthy();
    expect(api.created).toEqual([[2, "fix-login"]]);
  });

  it("shows the daemon's error when creating fails", async () => {
    render(<Harness api={fakeApi([ws({ slug: "dup" })])} />);
    fireEvent.click(await screen.findByRole("button", { name: "New workspace" }));
    fireEvent.change(await screen.findByLabelText("Workspace name"), { target: { value: "dup" } });
    fireEvent.click(screen.getByRole("button", { name: "Create workspace" }));
    expect((await screen.findByRole("alert")).textContent).toBe("a branch with this name already exists");
  });

  it("archives a workspace and drops it from the list", async () => {
    render(<Harness api={fakeApi([ws({ id: 1, slug: "task", state: "exited", exitCode: 0 })])} />);
    fireEvent.click(await screen.findByRole("button", { name: /task/ }));
    fireEvent.click(screen.getByRole("button", { name: "Archive task" }));
    expect(await screen.findByText("Pick a workspace to see its agent.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /task/ })).toBeNull();
  });

  it("offers Resume for a run tess cut off, when its agent can resume", async () => {
    render(
      <Harness
        api={fakeApi([
          ws({ id: 1, slug: "cut", state: "exited", ended: "interrupted", exitCode: -1, agent: "aider" }),
          ws({ id: 2, slug: "cut-plain", state: "exited", ended: "interrupted", exitCode: -1, agent: "plain" }),
        ])}
      />,
    );
    const input = await screen.findByRole("region", { name: "Needs your input" });
    expect(within(input).getAllByText("Cut off when tess stopped")).toHaveLength(2);
    expect(within(input).queryByRole("button", { name: "Resume cut-plain" })).toBeNull();

    fireEvent.click(await within(input).findByRole("button", { name: "Resume cut" }));
    const running = await screen.findByRole("region", { name: "Running" });
    expect(within(running).getByText("cut")).toBeTruthy();
    expect(screen.getByRole("heading", { level: 2, name: "cut" })).toBeTruthy();
  });
});
