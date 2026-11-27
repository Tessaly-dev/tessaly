// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { App } from "./App";
import type { AuthApi } from "./api";

afterEach(cleanup);

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
    render(<App load={healthy} auth={fakeAuth(true)} />);
    expect(await screen.findByText("Daemon ok, version v0.0.1")).toBeTruthy();
  });

  it("shows an error when the daemon is unreachable", async () => {
    render(<App load={async () => Promise.reject(new Error("boom"))} auth={fakeAuth(true)} />);
    expect(await screen.findByText("Daemon unreachable: boom")).toBeTruthy();
  });

  it("asks for the token when signed out, and signs in", async () => {
    const auth = fakeAuth(false);
    render(<App load={healthy} auth={auth} />);

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
    render(<App load={healthy} auth={auth} initialToken="good" />);
    expect(await screen.findByRole("button", { name: "Sign out" })).toBeTruthy();
    expect(auth.calls).toContain("signIn:good");
  });

  it("shows the sign-in form when the link token is wrong", async () => {
    render(<App load={healthy} auth={fakeAuth(false)} initialToken="stale" />);
    expect(await screen.findByLabelText("Token")).toBeTruthy();
  });

  it("signs out", async () => {
    render(<App load={healthy} auth={fakeAuth(true)} />);
    fireEvent.click(await screen.findByRole("button", { name: "Sign out" }));
    expect(await screen.findByLabelText("Token")).toBeTruthy();
  });
});
