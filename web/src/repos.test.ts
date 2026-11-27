// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it, vi } from "vitest";
import { reposApi } from "./repos";

function reply(status: number, body: unknown): typeof fetch {
  return vi.fn(async () => new Response(body === undefined ? null : JSON.stringify(body), { status })) as unknown as typeof fetch;
}

describe("reposApi", () => {
  it("lists repos", async () => {
    const repos = [{ id: 1, name: "app", path: "/src/app", addedAt: "2026-01-01T00:00:00Z" }];
    await expect(reposApi(reply(200, repos)).list()).resolves.toEqual(repos);
  });

  it("posts the path as JSON", async () => {
    const fetcher = reply(201, { id: 1, name: "app", path: "/src/app", addedAt: "" });
    await reposApi(fetcher).add("/src/app");
    expect(fetcher).toHaveBeenCalledWith("/api/v1/repos", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: "/src/app" }),
    });
  });

  it("uses the daemon's error message", async () => {
    await expect(reposApi(reply(409, { error: "this repository is already added" })).add("/x")).rejects.toThrow(
      "already added",
    );
    await expect(reposApi(reply(500, "oops")).list()).rejects.toThrow("listing repos failed: 500");
  });

  it("removes by id", async () => {
    const fetcher = reply(204, undefined);
    await reposApi(fetcher).remove(7);
    expect(fetcher).toHaveBeenCalledWith("/api/v1/repos/7", { method: "DELETE" });
  });
});
