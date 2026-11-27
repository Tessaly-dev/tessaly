// SPDX-License-Identifier: Apache-2.0
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { expect, test } from "@playwright/test";
import { startDaemon, type Daemon } from "./daemon";

let daemon: Daemon;
let fakeBin: string;
const savedPath = process.env.PATH;

// A stand-in gh on PATH records its arguments, prints a pull request URL and
// reports one passing and one failing check, so the test needs no GitHub
// account.
test.beforeAll(async () => {
  fakeBin = mkdtempSync(path.join(tmpdir(), "tess-e2e-gh-"));
  writeFileSync(
    path.join(fakeBin, "gh"),
    [
      "#!/bin/sh",
      'if [ "$2" = checks ]; then',
      `  echo '[{"name":"Go tests","bucket":"pass","link":"https://github.com/acme/demo/actions/runs/1"},{"name":"E2E","bucket":"fail"}]'`,
      "  exit 1",
      "fi",
      `printf '%s\\n' "$@" > "${fakeBin}/args"`,
      `cat > "${fakeBin}/stdin"`,
      "echo https://github.com/acme/demo/pull/42",
      "",
    ].join("\n"),
    { mode: 0o700 },
  );
  process.env.PATH = `${fakeBin}${path.delimiter}${savedPath}`;
  daemon = await startDaemon(47_904);
});

test.afterAll(async () => {
  process.env.PATH = savedPath;
  await daemon?.stop();
  rmSync(fakeBin, { recursive: true, force: true });
});

test("open a pull request from a workspace", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));

  // The repo pushes to a local bare remote.
  const remote = path.join(path.dirname(daemon.repo), "remote.git");
  execFileSync("git", ["init", "-q", "--bare", remote]);
  const git = (...args: string[]) => execFileSync("git", ["-C", daemon.repo, ...args], { encoding: "utf8" }).trim();
  git("remote", "add", "origin", remote);
  git("config", "user.name", "E2E");
  git("config", "user.email", "e2e@example.com");

  await page.goto(`${daemon.url}/#token=${daemon.token}`);
  await page.getByRole("tab", { name: "Repositories" }).click();
  await page.getByLabel("Repository folder").fill(daemon.repo);
  await page.getByRole("button", { name: "Add repository" }).click();
  await page.getByRole("tab", { name: "Home" }).click();
  await page.getByLabel("Workspace name").fill("add-readme");
  await page.getByRole("button", { name: "Create workspace" }).click();
  await expect(page.getByRole("region", { name: "Workspace" }).getByRole("heading", { name: "add-readme" })).toBeVisible();

  // Stand in for an agent: write a file into the workspace's worktree.
  const key = await page.evaluate(() => localStorage.getItem("tessaly.sessionKey"));
  const res = await page.request.get(`${daemon.url}/api/v1/workspaces`, { headers: { "X-Tessaly-Key": key ?? "" } });
  const [ws] = (await res.json()) as { path: string }[];
  writeFileSync(path.join(ws.path, "README.md"), "# Demo\n");

  const pr = page.getByRole("region", { name: "Pull request" });
  await expect(pr.getByLabel("Title")).toHaveValue("Add readme");
  await pr.getByLabel("Description").fill("Adds a README.");
  await pr.getByRole("button", { name: "Open pull request" }).click();
  await expect(pr.getByRole("status")).toHaveText("Pull request opened.");
  await expect(pr.getByRole("link", { name: "#42" })).toHaveAttribute("href", "https://github.com/acme/demo/pull/42");

  // The branch reached the remote with the commit, and gh got the title and body.
  expect(execFileSync("git", ["-C", remote, "log", "-1", "--format=%s", "tess/add-readme"], { encoding: "utf8" }).trim()).toBe(
    "Add readme",
  );
  expect(readFileSync(path.join(fakeBin, "args"), "utf8")).toBe(
    "pr\ncreate\n--head=tess/add-readme\n--title=Add readme\n--body-file=-\n",
  );
  expect(readFileSync(path.join(fakeBin, "stdin"), "utf8")).toBe("Adds a README.");

  // The checks show on the workspace and in the home list.
  const checks = pr.getByRole("region", { name: "Checks" });
  await expect(checks).toContainText("Some checks failed");
  await expect(checks.getByRole("listitem")).toHaveText(["Failed E2E", "Passed Go tests"]);
  await expect(page.getByRole("region", { name: "Ready to review" })).toContainText("Pull request open, checks failing");

  // Further changes push to the same pull request.
  await expect(pr.getByRole("button", { name: "Push changes" })).toBeVisible();

  expect(errors).toEqual([]);
});
