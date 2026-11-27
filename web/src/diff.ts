// SPDX-License-Identifier: Apache-2.0
import { apiFetch, errorFrom } from "./http";

// Diff is a workspace's changes since its base commit, as one unified diff.
export type Diff = { patch: string; truncated: boolean };

export type FileDiff = { path: string; added: number; removed: number; lines: string[] };

export async function fetchDiff(id: number, fetcher: typeof fetch = apiFetch): Promise<Diff> {
  const res = await fetcher(`/api/v1/workspaces/${encodeURIComponent(String(id))}/diff`, {
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw await errorFrom(res, "loading the changes");
  return (await res.json()) as Diff;
}

// splitPatch cuts a unified diff into one entry per file and counts its
// added and removed lines.
export function splitPatch(patch: string): FileDiff[] {
  const files: FileDiff[] = [];
  let file: FileDiff | null = null;
  let oldPath = "";
  // Header lines come before the first hunk. After it, "---" and "+++" are
  // removed and added lines.
  let inHunk = false;
  for (const line of patch.split("\n")) {
    if (line.startsWith("diff --git ")) {
      file = { path: gitLinePath(line.slice("diff --git ".length)), added: 0, removed: 0, lines: [line] };
      oldPath = "";
      inHunk = false;
      files.push(file);
      continue;
    }
    if (!file) continue;
    file.lines.push(line);
    if (!inHunk) {
      if (line.startsWith("@@")) inHunk = true;
      else if (line.startsWith("--- ")) oldPath = line.slice(4);
      else if (line.startsWith("+++ ")) {
        // A deleted file has "+++ /dev/null", so its name is on the "---" line.
        const newPath = line.slice(4);
        file.path = stripPrefix(newPath === "/dev/null" ? oldPath : newPath);
      } else if (line.startsWith("rename to ")) file.path = line.slice("rename to ".length);
    } else if (line.startsWith("+")) {
      file.added++;
    } else if (line.startsWith("-")) {
      file.removed++;
    }
  }
  // git ends the diff with a newline, which leaves one empty line at the end.
  if (file && file.lines[file.lines.length - 1] === "") file.lines.pop();
  return files;
}

// gitLinePath reads the name from "a/<path> b/<path>". A binary file has no
// "---" and "+++" lines, so this is the only name it gets.
function gitLinePath(rest: string): string {
  const n = (rest.length - 5) / 2;
  if (Number.isInteger(n) && n > 0 && rest.startsWith("a/") && rest.slice(2, 2 + n) === rest.slice(n + 5)) {
    return rest.slice(2, 2 + n);
  }
  return rest;
}

function stripPrefix(path: string): string {
  return path.startsWith("a/") || path.startsWith("b/") ? path.slice(2) : path;
}
