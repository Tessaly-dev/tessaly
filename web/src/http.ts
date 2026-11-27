// SPDX-License-Identifier: Apache-2.0

// errorFrom reads the {"error": "..."} body the daemon sends with a 4xx or 5xx.
export async function errorFrom(res: Response, action: string): Promise<Error> {
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

// The session key is the half of a sign-in that the cookie does not hold.
// Browsers send cookies for 127.0.0.1 to every port on it, but localStorage
// belongs to one origin, port included, so other local servers never see the
// key. It is also kept in memory for browsers that block storage.
const keyItem = "tessaly.sessionKey";
let memoryKey: string | null = null;

// keyHeader carries the session key on API requests.
export const keyHeader = "X-Tessaly-Key";

function storage(): Storage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

export function sessionKey(): string | null {
  if (memoryKey !== null) return memoryKey;
  try {
    return storage()?.getItem(keyItem) ?? null;
  } catch {
    return null;
  }
}

export function setSessionKey(key: string | null): void {
  memoryKey = key;
  try {
    if (key === null) storage()?.removeItem(keyItem);
    else storage()?.setItem(keyItem, key);
  } catch {
    // The key stays in memory for this page.
  }
}

// apiFetch is fetch with the session key added. Every API call uses it.
export const apiFetch: typeof fetch = (input, init) => {
  const headers = new Headers(init?.headers);
  const key = sessionKey();
  if (key) headers.set(keyHeader, key);
  return fetch(input, { ...init, headers });
};
