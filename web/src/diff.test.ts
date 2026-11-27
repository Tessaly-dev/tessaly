// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it, vi } from "vitest";
import { fetchDiff, splitPatch } from "./diff";

const patch = `diff --git a/keep.txt b/keep.txt
index 5626abf..814f4a4 100644
--- a/keep.txt
+++ b/keep.txt
@@ -1 +1,2 @@
 one
+two
diff --git a/gone.txt b/gone.txt
deleted file mode 100644
--- a/gone.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-a
--- not a header
diff --git a/new file.txt b/new file.txt
new file mode 100644
--- /dev/null
+++ b/new file.txt
@@ -0,0 +1 @@
+++ looks like a header
`;

describe("splitPatch", () => {
  it("splits files and counts lines", () => {
    const files = splitPatch(patch);
    expect(files.map((f) => [f.path, f.added, f.removed])).toEqual([
      ["keep.txt", 1, 0],
      ["gone.txt", 0, 2],
      ["new file.txt", 1, 0],
    ]);
    expect(files[0].lines[0]).toBe("diff --git a/keep.txt b/keep.txt");
    expect(files[2].lines.at(-1)).toBe("+++ looks like a header");
  });

  it("uses the new name of a renamed file", () => {
    const files = splitPatch("diff --git a/old.txt b/new.txt\nsimilarity index 100%\nrename from old.txt\nrename to new.txt\n");
    expect(files[0].path).toBe("new.txt");
  });

  it("names a binary file from the diff line", () => {
    const files = splitPatch("diff --git a/logo b.png b/logo b.png\nnew file mode 100644\nBinary files /dev/null and b/logo b.png differ\n");
    expect(files[0].path).toBe("logo b.png");
  });

  it("returns nothing for an empty patch", () => {
    expect(splitPatch("")).toEqual([]);
  });
});

describe("fetchDiff", () => {
  it("returns the diff", async () => {
    const fetcher = vi.fn(async (_url: RequestInfo | URL) => Response.json({ patch: "p", truncated: true }));
    expect(await fetchDiff(7, fetcher)).toEqual({ patch: "p", truncated: true });
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/workspaces/7/diff");
  });

  it("throws the daemon's error", async () => {
    const fetcher = async () => Response.json({ error: "no worktree" }, { status: 409 });
    await expect(fetchDiff(7, fetcher)).rejects.toThrow("no worktree");
  });
});
