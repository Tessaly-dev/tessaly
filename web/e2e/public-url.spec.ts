// SPDX-License-Identifier: Apache-2.0
import { expect, test } from "@playwright/test";
import { startDaemon, type Daemon } from "./daemon";

const port = 47_906;
const publicURL = `http://tess.test:${port}`;
let daemon: Daemon;

// The browser resolves tess.test to this machine, as a VPN name would. It
// skips any proxy from the environment, which would not know tess.test.
test.use({
  launchOptions: {
    args: ["--host-resolver-rules=MAP tess.test 127.0.0.1", "--no-proxy-server"],
    ...(process.env.PW_CHROMIUM_PATH ? { executablePath: process.env.PW_CHROMIUM_PATH } : {}),
  },
});

test.beforeAll(async () => {
  daemon = await startDaemon(port, `public_url = "${publicURL}"\n`);
});

test.afterAll(async () => {
  await daemon?.stop();
});

test("the UI and the terminal work at public_url", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));

  await page.goto(`${publicURL}/#token=${daemon.token}`);
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();

  await page.getByRole("tab", { name: "Repositories" }).click();
  await page.getByLabel("Repository folder").fill(daemon.repo);
  await page.getByRole("button", { name: "Add repository" }).click();
  await expect(page.getByRole("listitem").filter({ hasText: daemon.repo })).toBeVisible();

  // The terminal's WebSocket passes the Origin check on the public host.
  await page.getByRole("tab", { name: "Home" }).click();
  await page.getByLabel("Workspace name").fill("Public name");
  await page.getByRole("button", { name: "Create workspace" }).click();
  const detail = page.getByRole("region", { name: "Workspace" });
  await detail.getByLabel("Prompt").fill("hello from a VPN name");
  await detail.getByRole("button", { name: "Start agent" }).click();
  await expect(page.locator(".xterm-rows")).toContainText("task: hello from a VPN name");

  // Passkeys need https, so the Passkeys tab points to localhost.
  await page.getByRole("tab", { name: "Passkeys" }).click();
  await expect(page.getByRole("link", { name: `http://localhost:${port}` })).toBeVisible();
  expect(errors).toEqual([]);
});

test("other host names are still refused", async ({ request }) => {
  const res = await request.get(`http://127.0.0.1:${port}/healthz`, { headers: { Host: `evil.test:${port}` } });
  expect(res.status()).toBe(403);
});
