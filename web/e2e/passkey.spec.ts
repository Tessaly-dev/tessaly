// SPDX-License-Identifier: Apache-2.0
import { expect, test } from "@playwright/test";
import { startDaemon, type Daemon } from "./daemon";

const port = 47_905;
let daemon: Daemon;

test.beforeAll(async () => {
  daemon = await startDaemon(port);
});

test.afterAll(async () => {
  await daemon?.stop();
});

test("add a passkey, sign in with it, and remove it", async ({ page, context }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));

  // Chromium's virtual authenticator stands in for a fingerprint reader.
  const cdp = await context.newCDPSession(page);
  await cdp.send("WebAuthn.enable");
  await cdp.send("WebAuthn.addVirtualAuthenticator", {
    options: {
      protocol: "ctap2",
      transport: "internal",
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
    },
  });

  // Passkeys work only on localhost, so sign in there with the token link.
  const local = `http://localhost:${port}`;
  await page.goto(`${local}/#token=${daemon.token}`);
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();

  await page.getByRole("tab", { name: "Passkeys" }).click();
  await expect(page.getByText("No passkeys yet.")).toBeVisible();
  await page.getByLabel("Passkey name").fill("E2E key");
  await page.getByRole("button", { name: "Add a passkey" }).click();
  await expect(page.getByRole("listitem").filter({ hasText: "E2E key" })).toContainText("not used yet");

  // With a passkey, the token no longer signs in.
  const tokenLogin = await page.request.post(`${local}/api/v1/login`, {
    data: { token: daemon.token },
    headers: { Origin: local },
  });
  expect(tokenLogin.status()).toBe(403);

  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page.getByRole("button", { name: "Sign in with a passkey" })).toBeVisible();
  await expect(page.getByLabel("Token")).toHaveCount(0);
  await page.getByRole("button", { name: "Sign in with a passkey" }).click();
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();

  // On 127.0.0.1 the sign-in page points to localhost instead.
  const other = await context.newPage();
  await other.goto(daemon.url);
  await expect(other.getByRole("link", { name: local })).toBeVisible();
  await expect(other.getByRole("button", { name: "Sign in with a passkey" })).toHaveCount(0);
  await other.close();

  // A passkey sign-in is a fresh check, so removing needs no new prompt.
  await page.getByRole("tab", { name: "Passkeys" }).click();
  const row = page.getByRole("listitem").filter({ hasText: "E2E key" });
  await expect(row).toContainText("last used");
  await row.getByRole("button", { name: "Remove E2E key" }).click();
  // That passkey's own session ends with it.
  await expect(page.getByLabel("Token")).toBeVisible();

  expect(errors).toEqual([]);
});
