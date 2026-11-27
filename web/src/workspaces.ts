// SPDX-License-Identifier: Apache-2.0
import { errorFrom } from "./http";

export type Workspace = {
  id: number;
  repoId: number;
  slug: string;
  branch: string;
  base: string;
  path: string;
  state: string;
  error?: string;
  createdAt: string;
  agent?: string;
  exitCode?: number;
};

export type Agent = { name: string; command: string[] };

export type WorkspacesApi = {
  list(): Promise<Workspace[]>;
  create(repoId: number, name: string): Promise<Workspace>;
  archive(id: number): Promise<Workspace>;
  agents(): Promise<Agent[]>;
  start(id: number, agent: string, prompt: string): Promise<Workspace>;
  stop(id: number): Promise<Workspace>;
};

export function workspacesApi(fetcher: typeof fetch = fetch): WorkspacesApi {
  return {
    async list() {
      const res = await fetcher("/api/v1/workspaces", { headers: { Accept: "application/json" } });
      if (!res.ok) throw await errorFrom(res, "listing workspaces");
      return (await res.json()) as Workspace[];
    },

    async create(repoId: number, name: string) {
      const res = await fetcher("/api/v1/workspaces", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ repoId, name }),
      });
      if (!res.ok) throw await errorFrom(res, "creating the workspace");
      return (await res.json()) as Workspace;
    },

    async archive(id: number) {
      const res = await fetcher(`/api/v1/workspaces/${encodeURIComponent(String(id))}/archive`, { method: "POST" });
      if (!res.ok) throw await errorFrom(res, "archiving the workspace");
      return (await res.json()) as Workspace;
    },

    async agents() {
      const res = await fetcher("/api/v1/agents", { headers: { Accept: "application/json" } });
      if (!res.ok) throw await errorFrom(res, "listing agents");
      return (await res.json()) as Agent[];
    },

    async start(id: number, agent: string, prompt: string) {
      const res = await fetcher(`/api/v1/workspaces/${encodeURIComponent(String(id))}/start`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent, prompt }),
      });
      if (!res.ok) throw await errorFrom(res, "starting the agent");
      return (await res.json()) as Workspace;
    },

    async stop(id: number) {
      const res = await fetcher(`/api/v1/workspaces/${encodeURIComponent(String(id))}/stop`, { method: "POST" });
      if (!res.ok) throw await errorFrom(res, "stopping the agent");
      return (await res.json()) as Workspace;
    },
  };
}

// slugify turns a task title into a workspace name the daemon accepts:
// 1 to 48 of [a-z0-9-], starting with a letter or digit.
export function slugify(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+/, "")
    .slice(0, 48)
    .replace(/-+$/, "");
}
