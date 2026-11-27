// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { App } from "./App";

afterEach(cleanup);

describe("App", () => {
  it("shows the daemon version when healthy", async () => {
    render(<App load={async () => ({ status: "ok", version: "v0.0.1" })} />);
    expect(await screen.findByText("Daemon ok, version v0.0.1")).toBeTruthy();
  });

  it("shows an error when the daemon is unreachable", async () => {
    render(<App load={async () => Promise.reject(new Error("boom"))} />);
    expect(await screen.findByText("Daemon unreachable: boom")).toBeTruthy();
  });
});
