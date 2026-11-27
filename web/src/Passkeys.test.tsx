// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Passkeys, ResetNotice } from "./Passkeys";
import type { PasskeyApi, PasskeyList } from "./passkey";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const origin = window.location.origin;

function fakeApi(list: PasskeyList): PasskeyApi & { calls: string[] } {
  const calls: string[] = [];
  return {
    calls,
    loginOptions: async () => ({ token: true, passkey: false, passkeyOrigin: origin }),
    signIn: async () => undefined,
    list: async () => list,
    add: async (name) => {
      calls.push(`add:${name}`);
    },
    verify: async () => {
      calls.push("verify");
      list.fresh = true;
    },
    remove: async (id) => {
      calls.push(`remove:${id}`);
    },
    resetNotice: async () => null,
    dismissResetNotice: async () => undefined,
  };
}

const laptop = { id: 1, name: "Laptop", createdAt: "2026-11-27T10:00:00Z", lastUsedAt: null };

describe("Passkeys", () => {
  it("adds the first passkey without a check", async () => {
    vi.stubGlobal("PublicKeyCredential", function PublicKeyCredential() {});
    const api = fakeApi({ passkeys: [], fresh: false, origin });
    render(<Passkeys api={api} />);
    expect(await screen.findByText("No passkeys yet.")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Passkey name"), { target: { value: " Laptop " } });
    fireEvent.click(screen.getByRole("button", { name: "Add a passkey" }));
    await waitFor(() => expect(api.calls).toEqual(["add:Laptop"]));
  });

  it("asks for a passkey check before removing one", async () => {
    vi.stubGlobal("PublicKeyCredential", function PublicKeyCredential() {});
    const api = fakeApi({ passkeys: [laptop], fresh: false, origin });
    render(<Passkeys api={api} />);
    fireEvent.click(await screen.findByRole("button", { name: "Remove Laptop" }));
    await waitFor(() => expect(api.calls).toEqual(["verify", "remove:1"]));
  });

  it("links to the passkey address from anywhere else", async () => {
    vi.stubGlobal("PublicKeyCredential", function PublicKeyCredential() {});
    const api = fakeApi({ passkeys: [laptop], fresh: false, origin: "http://localhost:7777" });
    render(<Passkeys api={api} />);
    expect((await screen.findByRole("link", { name: "http://localhost:7777" })).getAttribute("href")).toBe(
      "http://localhost:7777",
    );
    expect(screen.queryByRole("button", { name: "Add a passkey" })).toBeNull();
    expect(screen.getByRole("button", { name: "Remove Laptop" })).toHaveProperty("disabled", true);
  });

  it("shows errors", async () => {
    vi.stubGlobal("PublicKeyCredential", function PublicKeyCredential() {});
    const api = fakeApi({ passkeys: [], fresh: false, origin });
    api.add = async () => Promise.reject(new Error("that passkey could not be added"));
    render(<Passkeys api={api} />);
    fireEvent.click(await screen.findByRole("button", { name: "Add a passkey" }));
    expect(await screen.findByRole("alert")).toHaveProperty("textContent", "that passkey could not be added");
  });
});

describe("ResetNotice", () => {
  it("warns after a reset until dismissed", async () => {
    const api = fakeApi({ passkeys: [], fresh: false, origin });
    api.resetNotice = async () => "2026-11-27T10:00:00Z";
    api.dismissResetNotice = async () => {
      api.calls.push("dismiss");
    };
    render(<ResetNotice api={api} />);
    expect((await screen.findByRole("alert")).textContent).toMatch(/Sign-in was reset from the host/);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(api.calls).toEqual(["dismiss"]);
  });

  it("shows nothing without a reset", async () => {
    const api = fakeApi({ passkeys: [], fresh: false, origin });
    render(<ResetNotice api={api} />);
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
  });
});
