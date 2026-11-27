// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it, vi } from "vitest";
import { slugify, workspacesApi } from "./workspaces";

function reply(status: number, body: unknown): typeof fetch {
  return vi.fn(async () => new Response(JSON.stringify(body), { status })) as unknown as typeof fetch;
}

describe("slugify", () => {
  it.each([
    ["Fix login bug", "fix-login-bug"],
    ["  --Hello, World!--  ", "hello-world"],
    ["x; rm -rf ~", "x-rm-rf"],
    ["日本", ""],
    ["a".repeat(60), "a".repeat(48)],
    ["a".repeat(47) + " b", "a".repeat(47)],
  ])("%s -> %s", (input, want) => {
    expect(slugify(input)).toBe(want);
  });
});

describe("workspacesApi", () => {
  it("creates with repo id and name", async () => {
    const fetcher = reply(201, { id: 1 });
    await workspacesApi(fetcher).create(3, "fix");
    expect(fetcher).toHaveBeenCalledWith("/api/v1/workspaces", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ repoId: 3, name: "fix" }),
    });
  });

  it("archives by id", async () => {
    const fetcher = reply(200, { id: 4, state: "archived" });
    await expect(workspacesApi(fetcher).archive(4)).resolves.toMatchObject({ state: "archived" });
    expect(fetcher).toHaveBeenCalledWith("/api/v1/workspaces/4/archive", { method: "POST" });
  });

  it("uses the daemon's error message", async () => {
    await expect(workspacesApi(reply(409, { error: "a branch with this name already exists" })).create(1, "x")).rejects.toThrow(
      "already exists",
    );
  });
});

describe("workspacesApi agent calls", () => {
  it("starts with agent and prompt", async () => {
    const fetcher = reply(200, { id: 2, state: "running" });
    await workspacesApi(fetcher).start(2, "aider", "fix");
    expect(fetcher).toHaveBeenCalledWith("/api/v1/workspaces/2/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ agent: "aider", prompt: "fix" }),
    });
  });
});
