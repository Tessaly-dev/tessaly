// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import { fetchHealth } from "./health";

function fakeFetch(status: number, body: unknown): typeof fetch {
  return async () => new Response(JSON.stringify(body), { status });
}

describe("fetchHealth", () => {
  it("returns the health body", async () => {
    await expect(fetchHealth(fakeFetch(200, { status: "ok", version: "dev" }))).resolves.toEqual({
      status: "ok",
      version: "dev",
    });
  });

  it("throws on a non-200 reply", async () => {
    await expect(fetchHealth(fakeFetch(403, {}))).rejects.toThrow("403");
  });

  it("throws on an unexpected body", async () => {
    await expect(fetchHealth(fakeFetch(200, { status: 1 }))).rejects.toThrow("unexpected body");
    await expect(fetchHealth(fakeFetch(200, null))).rejects.toThrow("unexpected body");
  });
});
