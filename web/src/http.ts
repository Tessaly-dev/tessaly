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
