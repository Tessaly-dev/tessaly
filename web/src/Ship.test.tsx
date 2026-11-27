// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Ship } from "./Ship";
import { prLabel, ship, titleFromSlug } from "./ship";
import type { Workspace } from "./workspaces";

afterEach(cleanup);

const ws: Workspace = {
  id: 4,
  repoId: 1,
  slug: "fix-login",
  branch: "tess/fix-login",
  base: "",
  path: "",
  state: "exited",
  createdAt: "",
};
const prUrl = "https://github.com/acme/app/pull/7";

describe("Ship", () => {
  it("opens a pull request with the title and description", async () => {
    const shipFn = vi.fn(async () => ({ ...ws, prUrl }));
    const onChange = vi.fn(async () => undefined);
    render(<Ship ws={ws} onChange={onChange} ship={shipFn} />);

    expect((screen.getByLabelText("Title") as HTMLInputElement).value).toBe("Fix login");
    fireEvent.change(screen.getByLabelText("Description"), { target: { value: "Closes #1" } });
    fireEvent.click(screen.getByRole("button", { name: "Open pull request" }));

    expect((await screen.findByRole("status")).textContent).toBe("Pull request opened.");
    expect(shipFn).toHaveBeenCalledWith(4, "Fix login", "Closes #1");
    expect(onChange).toHaveBeenCalled();
  });

  it("links the open pull request and pushes new commits to it", async () => {
    const shipFn = vi.fn(async () => ({ ...ws, prUrl }));
    render(<Ship ws={{ ...ws, prUrl }} onChange={async () => undefined} ship={shipFn} />);

    const link = screen.getByRole("link", { name: "#7" });
    expect(link.getAttribute("href")).toBe(prUrl);
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
    expect(screen.queryByLabelText("Description")).toBeNull();
    fireEvent.change(screen.getByLabelText("Commit message"), { target: { value: "Address review" } });
    fireEvent.click(screen.getByRole("button", { name: "Push changes" }));
    expect((await screen.findByRole("status")).textContent).toBe("Pushed to the pull request.");
    expect(shipFn).toHaveBeenCalledWith(4, "Address review", "");
  });

  it("shows why shipping failed", async () => {
    const shipFn = async () => Promise.reject(new Error("push: git push: no remote named origin"));
    render(<Ship ws={ws} onChange={async () => undefined} ship={shipFn} />);
    fireEvent.click(screen.getByRole("button", { name: "Open pull request" }));
    expect((await screen.findByRole("alert")).textContent).toBe("push: git push: no remote named origin");
  });

  it("waits for the agent to stop", () => {
    render(<Ship ws={{ ...ws, state: "running", prUrl }} onChange={async () => undefined} />);
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText("Stop the agent to ship its changes.")).toBeTruthy();
    expect(screen.getByRole("link", { name: "#7" })).toBeTruthy();
  });
});

describe("ship helpers", () => {
  it("posts the title and body", async () => {
    const fetcher = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit) => Response.json({ ...ws, prUrl }));
    expect((await ship(4, "T", "B", fetcher)).prUrl).toBe(prUrl);
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/workspaces/4/ship");
    expect(fetcher.mock.calls[0][1]?.body).toBe(JSON.stringify({ title: "T", body: "B" }));
  });

  it("throws the daemon's error", async () => {
    const fetcher = async () => Response.json({ error: "not signed in to gh" }, { status: 502 });
    await expect(ship(4, "T", "", fetcher)).rejects.toThrow("not signed in to gh");
  });

  it("makes titles and labels", () => {
    expect(titleFromSlug("fix-login-page")).toBe("Fix login page");
    expect(prLabel(prUrl)).toBe("#7");
    expect(prLabel("https://gitlab.com/a/b/-/merge_requests/3")).toBe("https://gitlab.com/a/b/-/merge_requests/3");
  });
});
