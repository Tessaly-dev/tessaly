// SPDX-License-Identifier: Apache-2.0
import { apiFetch, errorFrom } from "./http";
import type { Workspace } from "./workspaces";

// ship commits the workspace's changes, pushes its branch and opens a pull
// request, or pushes new commits to the one already open.
export async function ship(id: number, title: string, body: string, fetcher: typeof fetch = apiFetch): Promise<Workspace> {
  const res = await fetcher(`/api/v1/workspaces/${encodeURIComponent(String(id))}/ship`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title, body }),
  });
  if (!res.ok) throw await errorFrom(res, "shipping the workspace");
  return (await res.json()) as Workspace;
}

// titleFromSlug turns "fix-login" into "Fix login" as a starting title.
export function titleFromSlug(slug: string): string {
  const words = slug.replace(/-+/g, " ").trim();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

// prNumber returns "#7" for a URL ending in /pull/7, or the URL itself.
export function prLabel(url: string): string {
  const m = /\/pull\/(\d+)\/?$/.exec(url);
  return m ? `#${m[1]}` : url;
}
