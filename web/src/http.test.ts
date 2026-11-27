// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { apiFetch, keyHeader, sessionKey, setSessionKey } from "./http";

afterEach(() => {
  setSessionKey(null);
  vi.unstubAllGlobals();
});

describe("apiFetch", () => {
  function stubFetch() {
    const calls: Headers[] = [];
    vi.stubGlobal("fetch", async (_: RequestInfo | URL, init?: RequestInit) => {
      calls.push(new Headers(init?.headers));
      return new Response(null, { status: 204 });
    });
    return calls;
  }

  it("sends the session key and keeps other headers", async () => {
    const calls = stubFetch();
    setSessionKey("secret-key");
    await apiFetch("/api/v1/repos", { headers: { Accept: "application/json" } });
    expect(calls[0].get(keyHeader)).toBe("secret-key");
    expect(calls[0].get("Accept")).toBe("application/json");
  });

  it("sends no key when signed out", async () => {
    const calls = stubFetch();
    await apiFetch("/api/v1/session");
    expect(calls[0].has(keyHeader)).toBe(false);
  });
});

describe("sessionKey", () => {
  it("survives a reload through localStorage", () => {
    localStorage.setItem("tessaly.sessionKey", "from-storage");
    expect(sessionKey()).toBe("from-storage");
    localStorage.removeItem("tessaly.sessionKey");
  });
});
