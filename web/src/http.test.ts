// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { apiFetch, keyHeader, sessionKey, setSessionKey, signedOutEvent } from "./http";

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

describe("apiFetch on 401", () => {
  it("drops the key and tells the app the session ended", async () => {
    vi.stubGlobal("fetch", async () => new Response(null, { status: 401 }));
    const seen = vi.fn();
    window.addEventListener(signedOutEvent, seen);
    setSessionKey("old");
    await apiFetch("/api/v1/repos");
    window.removeEventListener(signedOutEvent, seen);
    expect(seen).toHaveBeenCalledTimes(1);
    expect(sessionKey()).toBeNull();
  });

  it("stays quiet when there was no session", async () => {
    vi.stubGlobal("fetch", async () => new Response(null, { status: 401 }));
    const seen = vi.fn();
    window.addEventListener(signedOutEvent, seen);
    await apiFetch("/api/v1/login", { method: "POST" });
    window.removeEventListener(signedOutEvent, seen);
    expect(seen).not.toHaveBeenCalled();
  });
});

describe("sessionKey", () => {
  it("survives a reload through localStorage", () => {
    localStorage.setItem("tessaly.sessionKey", "from-storage");
    expect(sessionKey()).toBe("from-storage");
    localStorage.removeItem("tessaly.sessionKey");
  });
});
