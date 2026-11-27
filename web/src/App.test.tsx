// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { App } from "./App";
import type { AuthApi } from "./api";
import type { ReposApi } from "./repos";
import type { WorkspacesApi } from "./workspaces";
import type { LoginOptions, PasskeyApi } from "./passkey";

afterEach(cleanup);

const noRepos: ReposApi = {
  list: async () => [],
  add: async () => Promise.reject(new Error("unused")),
  remove: async () => undefined,
};

const noWorkspaces: WorkspacesApi = {
  list: async () => [],
  create: async () => Promise.reject(new Error("unused")),
  archive: async () => Promise.reject(new Error("unused")),
  agents: async () => [],
  start: async () => Promise.reject(new Error("unused")),
  stop: async () => Promise.reject(new Error("unused")),
  resume: async () => Promise.reject(new Error("unused")),
};

const healthy = async () => ({ status: "ok", version: "v0.0.1" });

function fakeAuth(signedIn: boolean, token = "good"): AuthApi & { calls: string[] } {
  let state = signedIn;
  const calls: string[] = [];
  return {
    calls,
    isSignedIn: async () => state,
    signIn: async (t: string) => {
      calls.push(`signIn:${t}`);
      if (t !== token) throw new Error("That token is not right.");
      state = true;
    },
    signOut: async () => {
      calls.push("signOut");
      state = false;
    },
  };
}

describe("App", () => {
  it("shows the daemon version when healthy", async () => {
    render(<App repos={noRepos} workspaces={noWorkspaces} load={healthy} auth={fakeAuth(true)} />);
    expect(await screen.findByText("Daemon ok, version v0.0.1")).toBeTruthy();
  });

  it("shows an error when the daemon is unreachable", async () => {
    render(<App repos={noRepos} workspaces={noWorkspaces} load={async () => Promise.reject(new Error("boom"))} auth={fakeAuth(true)} />);
    expect(await screen.findByText("Daemon unreachable: boom")).toBeTruthy();
  });

  it("asks for the token when signed out, and signs in", async () => {
    const auth = fakeAuth(false);
    render(<App repos={noRepos} workspaces={noWorkspaces} load={healthy} auth={auth} />);

    const input = await screen.findByLabelText("Token");
    fireEvent.change(input, { target: { value: "bad" } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveProperty("textContent", "That token is not right.");

    fireEvent.change(input, { target: { value: " good " } });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("button", { name: "Sign out" })).toBeTruthy();
    expect(auth.calls).toEqual(["signIn:bad", "signIn:good"]);
  });

  it("signs in with the token from the link", async () => {
    const auth = fakeAuth(false);
    render(<App repos={noRepos} workspaces={noWorkspaces} load={healthy} auth={auth} initialToken="good" />);
    expect(await screen.findByRole("button", { name: "Sign out" })).toBeTruthy();
    expect(auth.calls).toContain("signIn:good");
  });

  it("shows the sign-in form when the link token is wrong", async () => {
    render(<App repos={noRepos} workspaces={noWorkspaces} load={healthy} auth={fakeAuth(false)} initialToken="stale" />);
    expect(await screen.findByLabelText("Token")).toBeTruthy();
  });

  it("signs out", async () => {
    render(<App repos={noRepos} workspaces={noWorkspaces} load={healthy} auth={fakeAuth(true)} />);
    fireEvent.click(await screen.findByRole("button", { name: "Sign out" }));
    expect(await screen.findByLabelText("Token")).toBeTruthy();
  });
});

function fakePasskeys(options: LoginOptions): PasskeyApi & { calls: string[] } {
  const calls: string[] = [];
  return {
    calls,
    loginOptions: async () => options,
    signIn: async () => {
      calls.push("signIn");
    },
    list: async () => ({ passkeys: [], fresh: false, origin: options.passkeyOrigin }),
    add: async () => undefined,
    verify: async () => undefined,
    remove: async () => undefined,
  };
}

describe("App passkey sign-in", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("signs in with a passkey when token sign-in is off", async () => {
    vi.stubGlobal("PublicKeyCredential", function PublicKeyCredential() {});
    const passkeys = fakePasskeys({ token: false, passkey: true, passkeyOrigin: window.location.origin });
    render(<App repos={noRepos} workspaces={noWorkspaces} load={healthy} auth={fakeAuth(false)} passkeys={passkeys} />);
    fireEvent.click(await screen.findByRole("button", { name: "Sign in with a passkey" }));
    expect(await screen.findByRole("button", { name: "Sign out" })).toBeTruthy();
    expect(passkeys.calls).toEqual(["signIn"]);
    expect(screen.queryByLabelText("Token")).toBeNull();
  });

  it("links to the passkey address from another address", async () => {
    vi.stubGlobal("PublicKeyCredential", function PublicKeyCredential() {});
    const passkeys = fakePasskeys({ token: false, passkey: true, passkeyOrigin: "http://localhost:7777" });
    render(<App repos={noRepos} workspaces={noWorkspaces} load={healthy} auth={fakeAuth(false)} passkeys={passkeys} />);
    expect(await screen.findByRole("link", { name: "http://localhost:7777" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Sign in with a passkey" })).toBeNull();
    expect(screen.getByText(/Token sign-in is off/)).toBeTruthy();
  });

  it("offers both while token sign-in stays on", async () => {
    vi.stubGlobal("PublicKeyCredential", function PublicKeyCredential() {});
    const passkeys = fakePasskeys({ token: true, passkey: true, passkeyOrigin: window.location.origin });
    render(<App repos={noRepos} workspaces={noWorkspaces} load={healthy} auth={fakeAuth(false)} passkeys={passkeys} />);
    expect(await screen.findByRole("button", { name: "Sign in with a passkey" })).toBeTruthy();
    expect(screen.getByLabelText("Token")).toBeTruthy();
  });
});

describe("App tabs", () => {
  it("switches between home, board and repositories", async () => {
    render(<App repos={noRepos} workspaces={noWorkspaces} load={healthy} auth={fakeAuth(true)} />);
    const home = await screen.findByRole("tab", { name: "Home" });
    expect(home.getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("tabpanel", { name: "Home" })).toBeTruthy();

    fireEvent.click(screen.getByRole("tab", { name: "Board" }));
    expect(screen.getByRole("tabpanel", { name: "Board" })).toBeTruthy();

    fireEvent.click(screen.getByRole("tab", { name: "Repositories" }));
    expect(screen.getByRole("tabpanel", { name: "Repositories" })).toBeTruthy();
    expect(screen.queryByRole("tabpanel", { name: "Home" })).toBeNull();
  });
});
