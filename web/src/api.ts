// SPDX-License-Identifier: Apache-2.0

// The daemon API client for sign-in. A session has two halves: an HttpOnly
// cookie that scripts never see, and a session key that apiFetch sends in a
// header. The daemon needs both.
import { apiFetch, setSessionKey } from "./http";

export type AuthApi = {
  isSignedIn(): Promise<boolean>;
  signIn(token: string): Promise<void>;
  signOut(): Promise<void>;
};

export function authApi(fetcher: typeof fetch = apiFetch): AuthApi {
  return {
    async isSignedIn() {
      const res = await fetcher("/api/v1/session", { headers: { Accept: "application/json" } });
      if (res.status === 401) return false;
      if (!res.ok) throw new Error(`session check failed: ${res.status}`);
      return true;
    },

    async signIn(token: string) {
      const res = await fetcher("/api/v1/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token }),
      });
      if (res.status === 401) throw new Error("That token is not right.");
      if (!res.ok) throw new Error(`sign-in failed: ${res.status}`);
      const body = (await res.json()) as { key?: unknown };
      if (typeof body.key !== "string" || body.key === "") throw new Error("sign-in reply has no session key");
      setSessionKey(body.key);
    },

    async signOut() {
      const res = await fetcher("/api/v1/logout", { method: "POST" });
      setSessionKey(null);
      if (!res.ok) throw new Error(`sign-out failed: ${res.status}`);
    },
  };
}

// takeTokenFromHash reads a token from a "#token=..." URL fragment and removes
// the fragment from the address bar, so the token does not stay in history.
export function takeTokenFromHash(loc: Location = window.location, hist: History = window.history): string | null {
  const params = new URLSearchParams(loc.hash.replace(/^#/, ""));
  const token = params.get("token");
  if (token === null) return null;
  hist.replaceState(null, "", loc.pathname + loc.search);
  return token;
}
