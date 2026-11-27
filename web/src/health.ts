// SPDX-License-Identifier: Apache-2.0

export type Health = { status: string; version: string };

// fetchHealth asks the daemon for its status. It throws on a non-200 reply or
// a body that does not match the expected shape.
export async function fetchHealth(fetcher: typeof fetch = fetch): Promise<Health> {
  const res = await fetcher("/healthz", { headers: { Accept: "application/json" } });
  if (!res.ok) throw new Error(`health check failed: ${res.status}`);
  const body: unknown = await res.json();
  if (!isHealth(body)) throw new Error("health check returned an unexpected body");
  return body;
}

function isHealth(v: unknown): v is Health {
  if (typeof v !== "object" || v === null) return false;
  const o = v as Record<string, unknown>;
  return typeof o.status === "string" && typeof o.version === "string";
}
