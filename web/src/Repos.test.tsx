// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Repos } from "./Repos";
import type { Repo, ReposApi } from "./repos";

afterEach(cleanup);

function fakeApi(): ReposApi {
  let repos: Repo[] = [];
  let next = 1;
  return {
    list: async () => repos,
    add: async (path: string) => {
      if (!path.startsWith("/")) throw new Error("path must be absolute");
      const repo = { id: next++, name: path.split("/").pop() ?? "repo", path, addedAt: "" };
      repos = [...repos, repo];
      return repo;
    },
    remove: async (id: number) => {
      repos = repos.filter((r) => r.id !== id);
    },
  };
}

describe("Repos", () => {
  it("adds, lists and removes repos", async () => {
    render(<Repos api={fakeApi()} />);
    expect(await screen.findByText(/No repositories yet/)).toBeTruthy();

    fireEvent.change(screen.getByLabelText("Repository folder"), { target: { value: " /src/app " } });
    fireEvent.click(screen.getByRole("button", { name: "Add repository" }));
    expect(await screen.findByText("/src/app")).toBeTruthy();
    expect((screen.getByLabelText("Repository folder") as HTMLInputElement).value).toBe("");

    fireEvent.click(screen.getByRole("button", { name: "Remove app" }));
    expect(await screen.findByText(/No repositories yet/)).toBeTruthy();
  });

  it("shows the daemon's error", async () => {
    render(<Repos api={fakeApi()} />);
    fireEvent.change(await screen.findByLabelText("Repository folder"), { target: { value: "relative" } });
    fireEvent.click(screen.getByRole("button", { name: "Add repository" }));
    expect((await screen.findByRole("alert")).textContent).toBe("path must be absolute");
  });
});
