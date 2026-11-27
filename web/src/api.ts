// SPDX-License-Identifier: Apache-2.0

// The daemon API client for sign-in. Every call sends cookies for this origin
// only, and the server sets them HttpOnly, so scripts never see the session.

export type AuthApi = {
  isSignedIn(): Promise<boolean>;
  signIn(token: string): Promise<void>;
  signOut(): Promise<void>;
};

export function authApi(fetcher: typeof fetch = fetch): AuthApi {
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
    },

    async signOut() {
      const res = await fetcher("/api/v1/logout", { method: "POST" });
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
