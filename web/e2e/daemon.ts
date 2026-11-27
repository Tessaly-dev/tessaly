// SPDX-License-Identifier: Apache-2.0
import { spawn, execFileSync, type ChildProcess } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

// Daemon is a tess serve process with its own data folder and a git repo.
export type Daemon = {
  url: string;
  token: string;
  // home is the daemon's data folder, TESSALY_HOME.
  home: string;
  repo: string;
  // restart stops tess serve and starts it again with the same data folder,
  // so agents that ran are cut off, as in a real restart.
  restart(): Promise<void>;
  stop(): Promise<void>;
};

export const binary = path.resolve(import.meta.dirname, "../../bin/tess");

// startDaemon runs bin/tess on port with a fresh TESSALY_HOME, a config that
// defines the agents the tests use, and a repo with one commit.
export async function startDaemon(port: number): Promise<Daemon> {
  if (!existsSync(binary)) throw new Error(`${binary} not found. Run "make build" first.`);
  const root = mkdtempSync(path.join(tmpdir(), "tess-e2e-"));
  const home = path.join(root, "home");
  mkdirSync(home, { mode: 0o700 });

  // The echo agent prints its task, then echoes what you type, like a
  // simple interactive agent. No shell parses the prompt: it is "$1".
  writeFileSync(
    path.join(home, "config.toml"),
    `[agents.echo]\ncommand = ["sh", "-c", "echo \\"task: $1\\"; exec cat", "echo-agent", "{prompt}"]\n` +
      `resume = ["sh", "-c", "echo resumed; exec cat"]\n`,
    { mode: 0o600 },
  );

  const repo = path.join(root, "demo-repo");
  const git = (...args: string[]) =>
    execFileSync("git", ["-c", "user.name=E2E", "-c", "user.email=e2e@example.com", ...args], { cwd: repo });
  mkdirSync(repo);
  git("init", "-q");
  git("commit", "-q", "--allow-empty", "-m", "first");

  const url = `http://127.0.0.1:${port}`;
  let proc = await serve(port, home, url);
  const token = readFileSync(path.join(home, "token"), "utf8").trim();

  return {
    url,
    token,
    home,
    repo,
    async restart() {
      await stopProcess(proc);
      proc = await serve(port, home, url);
    },
    async stop() {
      await stopProcess(proc);
      rmSync(root, { recursive: true, force: true });
    },
  };
}

// serve starts tess serve on port with home as its data folder, and waits
// until it answers.
async function serve(port: number, home: string, url: string): Promise<ChildProcess> {
  const proc = spawn(binary, ["serve", "--port", String(port)], {
    env: { ...process.env, TESSALY_HOME: home },
    stdio: ["ignore", "pipe", "pipe"],
  });
  let log = "";
  proc.stdout?.on("data", (b) => (log += b));
  proc.stderr?.on("data", (b) => (log += b));
  await waitHealthy(url, proc, () => log);
  return proc;
}

async function waitHealthy(url: string, proc: ChildProcess, log: () => string) {
  const deadline = Date.now() + 10_000;
  while (Date.now() < deadline) {
    if (proc.exitCode !== null) throw new Error(`tess exited early:\n${log()}`);
    try {
      if ((await fetch(`${url}/healthz`)).ok) return;
    } catch {
      // Not listening yet.
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`tess did not become healthy:\n${log()}`);
}

function stopProcess(proc: ChildProcess): Promise<void> {
  if (proc.exitCode !== null) return Promise.resolve();
  return new Promise((resolve) => {
    proc.once("exit", () => resolve());
    proc.kill("SIGTERM");
  });
}
