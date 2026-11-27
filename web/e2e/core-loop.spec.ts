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

test("sign in, run an agent from the home view, and use its terminal", async ({ page }) => {
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

  // Browsers send the session cookie to every port on 127.0.0.1, so another
  // local server would get it. The cookie alone must not reach the API. The
  // session key from this origin's storage, added to the same cookie, does.
  const api = `${daemon.url}/api/v1/session`;
  expect((await page.request.get(api)).status()).toBe(401);
  const key = await page.evaluate(() => localStorage.getItem("tessaly.sessionKey"));
  expect(key).toBeTruthy();
  expect((await page.request.get(api, { headers: { "X-Tessaly-Key": key ?? "" } })).status()).toBe(200);

  // The session survives a reload.
  await page.reload();
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();

  // Add the repo on the Repositories tab.
  await page.getByRole("tab", { name: "Repositories" }).click();
  await page.getByLabel("Repository folder").fill(daemon.repo);
  await page.getByRole("button", { name: "Add repository" }).click();
  await expect(page.getByRole("listitem").filter({ hasText: daemon.repo })).toContainText("demo-repo");

  // Create a workspace from the home view. It opens beside the list.
  await page.getByRole("tab", { name: "Home" }).click();
  await page.getByLabel("Workspace name").fill("Fix login");
  await expect(page.getByText("Branch: tess/fix-login")).toBeVisible();
  await page.getByRole("button", { name: "Create workspace" }).click();
  const group = (name: string) => page.getByRole("region", { name });
  await expect(group("Not started")).toContainText("fix-login");
  const detail = group("Workspace");
  await expect(detail.getByRole("heading", { name: "fix-login" })).toBeVisible();

  // Start the agent. The prompt reaches it as one argument, shell syntax and all.
  await detail.getByLabel("Prompt").fill("fix the login; rm -rf ~");
  await detail.getByRole("button", { name: "Start agent" }).click();
  await expect(group("Running")).toContainText("fix-login");
  const terminal = page.locator(".xterm-rows");
  await expect(terminal).toContainText("task: fix the login; rm -rf ~");

  // The echo agent now waits quietly for input, so it moves to the top group.
  await expect(group("Needs your input")).toContainText("fix-login", { timeout: 15_000 });

  // Keystrokes reach the agent, and its output comes back live.
  await page.locator(".xterm").click();
  await page.keyboard.type("hello from the browser");
  await page.keyboard.press("Enter");
  await expect(terminal).toContainText("hello from the browser");

  // Stop the agent. It moves to review, and the transcript stays visible.
  await detail.getByRole("button", { name: "Stop agent" }).click();
  await expect(group("Ready to review")).toContainText("Stopped by you");
  await expect(detail.getByText("Last run exited with code -1.")).toBeVisible();
  await expect(terminal).toContainText("task: fix the login");

  // The board shows the same workspace in its stage, and a click opens it on Home.
  await page.getByRole("tab", { name: "Board" }).click();
  await expect(group("Not started")).not.toContainText("fix-login");
  await group("Ready to review").getByRole("button", { name: /fix-login/ }).click();
  await expect(page.getByRole("tab", { name: "Home" })).toHaveAttribute("aria-selected", "true");
  await expect(detail.getByRole("heading", { name: "fix-login" })).toBeVisible();

  // Archive the workspace. It leaves the home list and shows as archived on the board.
  await detail.getByRole("button", { name: "Archive fix-login" }).click();
  await expect(page.getByText("Pick a workspace to see its agent.")).toBeVisible();
  await expect(page.getByRole("button", { name: /fix-login/ })).toHaveCount(0);
  await page.getByRole("tab", { name: "Board" }).click();
  await expect(group("Archived")).toContainText("fix-login");

  // Sign out.
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page.getByLabel("Token")).toBeVisible();
  expect(await page.evaluate(() => localStorage.getItem("tessaly.sessionKey"))).toBeNull();

  expect(errors).toEqual([]);
});
