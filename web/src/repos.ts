// SPDX-License-Identifier: Apache-2.0

export type Repo = { id: number; name: string; path: string; addedAt: string };

export type ReposApi = {
  list(): Promise<Repo[]>;
  add(path: string): Promise<Repo>;
  remove(id: number): Promise<void>;
};

// errorFrom reads the {"error": "..."} body the daemon sends with a 4xx or 5xx.
async function errorFrom(res: Response, action: string): Promise<Error> {
  try {
    const body: unknown = await res.json();
    if (typeof body === "object" && body !== null && typeof (body as { error?: unknown }).error === "string") {
      return new Error((body as { error: string }).error);
    }
  } catch {
    // Fall through to the generic message.
  }
  return new Error(`${action} failed: ${res.status}`);
}

export function reposApi(fetcher: typeof fetch = fetch): ReposApi {
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
