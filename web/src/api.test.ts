// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it, vi } from "vitest";
import { authApi, takeTokenFromHash } from "./api";

function reply(status: number): typeof fetch {
  return vi.fn(async () => new Response(null, { status })) as unknown as typeof fetch;
}

describe("authApi", () => {
  it("reports the session state", async () => {
    await expect(authApi(reply(200)).isSignedIn()).resolves.toBe(true);
    await expect(authApi(reply(401)).isSignedIn()).resolves.toBe(false);
    await expect(authApi(reply(500)).isSignedIn()).rejects.toThrow("500");
  });

  it("posts the token as JSON", async () => {
    const fetcher = reply(204);
    await authApi(fetcher).signIn("abc");
    expect(fetcher).toHaveBeenCalledWith("/api/v1/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token: "abc" }),
    });
  });

  it("explains a wrong token", async () => {
    await expect(authApi(reply(401)).signIn("x")).rejects.toThrow("not right");
    await expect(authApi(reply(403)).signIn("x")).rejects.toThrow("403");
  });

  it("signs out", async () => {
    await expect(authApi(reply(204)).signOut()).resolves.toBeUndefined();
    await expect(authApi(reply(500)).signOut()).rejects.toThrow("500");
  });
});

describe("takeTokenFromHash", () => {
  function fakeLocation(hash: string) {
    const replaceState = vi.fn();
    const loc = { hash, pathname: "/", search: "?a=1" } as Location;
    return { loc, hist: { replaceState } as unknown as History, replaceState };
  }

  it("reads the token and clears the fragment", () => {
    const { loc, hist, replaceState } = fakeLocation("#token=abc_-123");
    expect(takeTokenFromHash(loc, hist)).toBe("abc_-123");
    expect(replaceState).toHaveBeenCalledWith(null, "", "/?a=1");
  });

  it("leaves other fragments alone", () => {
    const { loc, hist, replaceState } = fakeLocation("#section");
    expect(takeTokenFromHash(loc, hist)).toBeNull();
    expect(replaceState).not.toHaveBeenCalled();
  });
});
