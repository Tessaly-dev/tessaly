// SPDX-License-Identifier: Apache-2.0
import { expect, test, type Page } from "@playwright/test";
import { startDaemon, type Daemon } from "./daemon";

let daemon: Daemon;

test.beforeAll(async () => {
  daemon = await startDaemon(47_901);
});

test.afterAll(async () => {
  await daemon?.stop();
});

// failOnPageErrors collects console errors and uncaught exceptions, which
// catches CSP violations and broken scripts. The browser also logs every 401,
// and those are expected while signed out, so they are left out.
function failOnPageErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error" && !m.text().includes("status of 401")) errors.push(m.text());
  });
  page.on("pageerror", (e) => errors.push(e.message));
  return errors;
}

test("sign in, run an agent in a workspace, and use its terminal", async ({ page }) => {
  const errors = failOnPageErrors(page);

  // A wrong token is refused.
  await page.goto(daemon.url);
  await page.getByLabel("Token").fill("not-the-token");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toHaveText("That token is not right.");

  // The link from "tess serve" signs in and clears the token from the URL.
  await page.goto(`${daemon.url}/#token=${daemon.token}`);
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
  expect(page.url()).toBe(`${daemon.url}/`);

  // Add the repo.
  await page.getByLabel("Repository folder").fill(daemon.repo);
  await page.getByRole("button", { name: "Add repository" }).click();
  await expect(page.getByRole("listitem").filter({ hasText: daemon.repo })).toContainText("demo-repo");

  // Create a workspace.
  await page.getByLabel("Workspace name").fill("Fix login");
  await expect(page.getByText("Branch: tess/fix-login")).toBeVisible();
  await page.getByRole("button", { name: "Create workspace" }).click();
  const row = page.getByRole("listitem").filter({ hasText: "fix-login" });
  await expect(row.locator(".state")).toHaveText("ready");

  // Start the agent. The prompt reaches it as one argument, shell syntax and all.
  await row.getByRole("button", { name: "Agent for fix-login" }).click();
  await page.getByLabel("Prompt").fill("fix the login; rm -rf ~");
  await page.getByRole("button", { name: "Start agent" }).click();
  await expect(row.locator(".state")).toHaveText("running");
  const terminal = page.locator(".xterm-rows");
  await expect(terminal).toContainText("task: fix the login; rm -rf ~");

  // Keystrokes reach the agent, and its output comes back live.
  await page.locator(".xterm").click();
  await page.keyboard.type("hello from the browser");
  await page.keyboard.press("Enter");
  await expect(terminal).toContainText("hello from the browser");

  // Stop the agent. The transcript stays visible.
  await page.getByRole("button", { name: "Stop agent" }).click();
  await expect(row.locator(".state")).toHaveText("exited");
  await expect(row.getByText("Last run exited with code -1.")).toBeVisible();
  await expect(terminal).toContainText("task: fix the login");

  // Archive the workspace.
  await row.getByRole("button", { name: "Archive fix-login" }).click();
  await expect(row.locator(".state")).toHaveText("archived");

  // Sign out.
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page.getByLabel("Token")).toBeVisible();

  expect(errors).toEqual([]);
});
