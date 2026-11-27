// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { AgentPanel } from "./AgentPanel";
import type { Workspace, WorkspacesApi } from "./workspaces";

afterEach(cleanup);

const ws: Workspace = { id: 1, repoId: 1, slug: "fix", branch: "tess/fix", base: "", path: "", state: "ready", createdAt: "" };

function fakeApi(over: Partial<WorkspacesApi> = {}): WorkspacesApi {
  return {
    list: async () => [],
    create: async () => ws,
    archive: async () => ws,
    agents: async () => [],
    start: vi.fn(async () => ({ ...ws, state: "running" })),
    stop: vi.fn(async () => ({ ...ws, state: "exited" })),
    output: async () => "<b>not html</b>",
    ...over,
  };
}

const agents = [
  { name: "aider", command: ["aider"] },
  { name: "codex", command: ["codex"] },
];

describe("AgentPanel", () => {
  it("starts the chosen agent with the prompt", async () => {
    const api = fakeApi();
    const onChange = vi.fn(async () => undefined);
    render(<AgentPanel ws={ws} agents={agents} api={api} onChange={onChange} />);

    fireEvent.change(screen.getByLabelText("Agent"), { target: { value: "codex" } });
    fireEvent.change(screen.getByLabelText("Prompt"), { target: { value: "fix the login" } });
    fireEvent.click(screen.getByRole("button", { name: "Start agent" }));

    await vi.waitFor(() => expect(onChange).toHaveBeenCalled());
    expect(api.start).toHaveBeenCalledWith(1, "codex", "fix the login");
  });

  it("shows output as text, never as HTML", async () => {
    render(<AgentPanel ws={ws} agents={agents} api={fakeApi()} onChange={async () => undefined} />);
    const out = await screen.findByLabelText("Agent output");
    expect(out.textContent).toBe("<b>not html</b>");
    expect(out.querySelector("b")).toBeNull();
  });

  it("stops a running agent", async () => {
    const api = fakeApi();
    render(<AgentPanel ws={{ ...ws, state: "running" }} agents={agents} api={api} onChange={async () => undefined} />);
    fireEvent.click(screen.getByRole("button", { name: "Stop agent" }));
    await vi.waitFor(() => expect(api.stop).toHaveBeenCalledWith(1));
  });

  it("polls output while running", async () => {
    let n = 0;
    const api = fakeApi({ output: async () => `line ${++n}` });
    render(<AgentPanel ws={{ ...ws, state: "running" }} agents={agents} api={api} onChange={async () => undefined} pollMs={10} />);
    await screen.findByText("line 3");
  });

  it("explains how to add an agent", () => {
    render(<AgentPanel ws={ws} agents={[]} api={fakeApi()} onChange={async () => undefined} />);
    expect(screen.getByText(/No agents set up/)).toBeTruthy();
  });

  it("shows the daemon's error", async () => {
    const api = fakeApi({ start: async () => Promise.reject(new Error("no such agent")) });
    render(<AgentPanel ws={ws} agents={agents} api={api} onChange={async () => undefined} />);
    fireEvent.click(screen.getByRole("button", { name: "Start agent" }));
    expect((await screen.findByRole("alert")).textContent).toBe("no such agent");
  });
});
