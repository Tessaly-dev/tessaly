// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Changes } from "./Changes";

afterEach(cleanup);

const patch = `diff --git a/login.go b/login.go
new file mode 100644
--- /dev/null
+++ b/login.go
@@ -0,0 +1,2 @@
+package login
+<script>alert(1)</script>
`;

describe("Changes", () => {
  it("loads the diff when asked and lists each file", async () => {
    const load = vi.fn(async () => ({ patch, truncated: false }));
    render(<Changes workspaceId={3} load={load} />);
    expect(load).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Review changes" }));
    expect(await screen.findByText("login.go")).toBeTruthy();
    expect(load).toHaveBeenCalledWith(3);
    expect(screen.getByText(/1 file changed/)).toBeTruthy();
    // Diff text shows as text, never as markup.
    expect(screen.getByText("+<script>alert(1)</script>")).toBeTruthy();
    expect(document.querySelector("script")).toBeNull();
    expect(screen.getByRole("button", { name: "Refresh changes" })).toBeTruthy();
  });

  it("says when there are no changes", async () => {
    render(<Changes workspaceId={3} load={async () => ({ patch: "", truncated: false })} />);
    fireEvent.click(screen.getByRole("button", { name: "Review changes" }));
    expect(await screen.findByText("No changes yet.")).toBeTruthy();
  });

  it("says when the diff was cut", async () => {
    render(<Changes workspaceId={3} load={async () => ({ patch, truncated: true })} />);
    fireEvent.click(screen.getByRole("button", { name: "Review changes" }));
    expect(await screen.findByText(/too big to show in full/)).toBeTruthy();
  });

  it("shows an error", async () => {
    render(<Changes workspaceId={3} load={async () => Promise.reject(new Error("no worktree"))} />);
    fireEvent.click(screen.getByRole("button", { name: "Review changes" }));
    expect((await screen.findByRole("alert")).textContent).toBe("no worktree");
  });
});

describe("Changes lines", () => {
  it("hides git headers but keeps hunk lines that look like them", async () => {
    const p = "diff --git a/a.txt b/a.txt\nindex 1..2 100644\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n--- old\n+++ new\n";
    render(<Changes workspaceId={1} load={async () => ({ patch: p, truncated: false })} />);
    fireEvent.click(screen.getByRole("button", { name: "Review changes" }));
    expect((await screen.findByText("+++ new")).className).toBe("add");
    expect(screen.getByText("--- old").className).toBe("del");
    expect(screen.queryByText("+++ b/a.txt")).toBeNull();
    expect(screen.queryByText(/^index /)).toBeNull();
  });

  it("shows what happened to a binary file", async () => {
    const p = "diff --git a/logo.png b/logo.png\nnew file mode 100644\nindex 0000000..1\nBinary files /dev/null and b/logo.png differ\n";
    render(<Changes workspaceId={1} load={async () => ({ patch: p, truncated: false })} />);
    fireEvent.click(screen.getByRole("button", { name: "Review changes" }));
    expect(await screen.findByText("Binary files /dev/null and b/logo.png differ")).toBeTruthy();
    expect(screen.getByText("new file mode 100644")).toBeTruthy();
  });
});
