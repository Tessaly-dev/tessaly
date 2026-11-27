// SPDX-License-Identifier: Apache-2.0
import { expect, test } from "@playwright/test";
import { startDaemon, type Daemon } from "./daemon";

let daemon: Daemon;

test.beforeAll(async () => {
  daemon = await startDaemon(47_902);
});

test.afterAll(async () => {
  await daemon?.stop();
});

test("resume an agent that a restart of tess cut off", async ({ page }) => {
  const errors: string[] = [];
  // A restart ends the session, so 401s and a dropped terminal are expected.
  page.on("pageerror", (e) => errors.push(e.message));

  await page.goto(`${daemon.url}/#token=${daemon.token}`);
  await page.getByRole("tab", { name: "Repositories" }).click();
  await page.getByLabel("Repository folder").fill(daemon.repo);
  await page.getByRole("button", { name: "Add repository" }).click();
  await expect(page.getByRole("listitem").filter({ hasText: daemon.repo })).toBeVisible();

  await page.getByRole("tab", { name: "Home" }).click();
  await page.getByLabel("Workspace name").fill("long-task");
  await page.getByRole("button", { name: "Create workspace" }).click();
  const detail = page.getByRole("region", { name: "Workspace" });
  await detail.getByLabel("Prompt").fill("keep going");
  await detail.getByRole("button", { name: "Start agent" }).click();
  await expect(page.locator(".xterm-rows")).toContainText("task: keep going");

  // Restart tess. Nothing runs again until you press Resume.
  await daemon.restart();
  await page.goto(`${daemon.url}/#token=${daemon.token}`);
  const input = page.getByRole("region", { name: "Needs your input" });
  await expect(input).toContainText("Cut off when tess stopped");
  await expect(page.getByRole("region", { name: "Running" })).toHaveCount(0);

  await input.getByRole("button", { name: "Resume long-task" }).click();
  await expect(page.getByRole("region", { name: "Running" })).toContainText("long-task");
  await expect(page.locator(".xterm-rows")).toContainText("resumed");

  expect(errors).toEqual([]);
});
