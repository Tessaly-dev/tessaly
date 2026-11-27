// SPDX-License-Identifier: Apache-2.0
import { writeFileSync } from "node:fs";
import path from "node:path";
import { expect, test } from "@playwright/test";
import { startDaemon, type Daemon } from "./daemon";

let daemon: Daemon;

test.beforeAll(async () => {
  daemon = await startDaemon(47_903);
});

test.afterAll(async () => {
  await daemon?.stop();
});

test("review a workspace's changes", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));

  await page.goto(`${daemon.url}/#token=${daemon.token}`);
  await page.getByRole("tab", { name: "Repositories" }).click();
  await page.getByLabel("Repository folder").fill(daemon.repo);
  await page.getByRole("button", { name: "Add repository" }).click();
  await page.getByRole("tab", { name: "Home" }).click();
  await page.getByLabel("Workspace name").fill("add-readme");
  await page.getByRole("button", { name: "Create workspace" }).click();

  const changes = page.getByRole("region", { name: "Changes" });
  await changes.getByRole("button", { name: "Review changes" }).click();
  await expect(changes.getByText("No changes yet.")).toBeVisible();

  // Stand in for an agent: write a file into the workspace's worktree.
  const key = await page.evaluate(() => localStorage.getItem("tessaly.sessionKey"));
  const res = await page.request.get(`${daemon.url}/api/v1/workspaces`, { headers: { "X-Tessaly-Key": key ?? "" } });
  const [ws] = (await res.json()) as { path: string }[];
  writeFileSync(path.join(ws.path, "README.md"), "# Demo\n\nHello from the agent.\n");

  await changes.getByRole("button", { name: "Refresh changes" }).click();
  await expect(changes.getByText("1 file changed")).toBeVisible();
  await expect(changes.getByText("README.md", { exact: true })).toBeVisible();
  await expect(changes.getByText("+Hello from the agent.")).toBeVisible();

  expect(errors).toEqual([]);
});
