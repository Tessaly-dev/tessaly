// SPDX-License-Identifier: Apache-2.0

import { apiFetch, errorFrom } from "./http";

export type Repo = { id: number; name: string; path: string; addedAt: string };

export type ReposApi = {
  list(): Promise<Repo[]>;
  add(path: string): Promise<Repo>;
  remove(id: number): Promise<void>;
};

export function reposApi(fetcher: typeof fetch = apiFetch): ReposApi {
  return {
    async list() {
      const res = await fetcher("/api/v1/repos", { headers: { Accept: "application/json" } });
      if (!res.ok) throw await errorFrom(res, "listing repos");
      return (await res.json()) as Repo[];
    },

    async add(path: string) {
      const res = await fetcher("/api/v1/repos", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path }),
      });
      if (!res.ok) throw await errorFrom(res, "adding the repo");
      return (await res.json()) as Repo;
    },

    async remove(id: number) {
      const res = await fetcher(`/api/v1/repos/${encodeURIComponent(String(id))}`, { method: "DELETE" });
      if (!res.ok) throw await errorFrom(res, "removing the repo");
    },
  };
}
