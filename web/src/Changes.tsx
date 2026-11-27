// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { fetchDiff, splitPatch, type Diff } from "./diff";

type Props = {
  workspaceId: number;
  load?: (id: number) => Promise<Diff>;
};

// Changes shows what the workspace changed since its base commit, one
// collapsible section per file. It loads on demand, since a diff can be big.
export function Changes({ workspaceId, load = fetchDiff }: Props) {
  const [diff, setDiff] = useState<Diff | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const files = diff ? splitPatch(diff.patch) : [];
  const added = files.reduce((n, f) => n + f.added, 0);
  const removed = files.reduce((n, f) => n + f.removed, 0);

  async function refresh() {
    setLoading(true);
    setError(null);
    try {
      setDiff(await load(workspaceId));
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
    setLoading(false);
  }

  return (
    <section aria-label="Changes" className="changes">
      <button type="button" onClick={() => void refresh()} disabled={loading}>
        {diff ? "Refresh changes" : "Review changes"}
      </button>
      {error && <p role="alert">{error}</p>}
      {diff && files.length === 0 && <p className="hint">No changes yet.</p>}
      {files.length > 0 && (
        <p className="hint">
          {files.length} {files.length === 1 ? "file" : "files"} changed, <span className="add">+{added}</span>{" "}
          <span className="del">-{removed}</span>
        </p>
      )}
      {diff?.truncated && <p className="hint">The diff is too big to show in full. Only the start shows.</p>}
      {files.map((f, i) => (
        <details key={`${i}-${f.path}`} open={files.length <= 10}>
          <summary>
            <code>{f.path}</code> <span className="add">+{f.added}</span> <span className="del">-{f.removed}</span>
          </summary>
          <pre className="patch">
            {shown(f.lines).map((line, j) => (
              <span key={j} className={lineClass(line)}>
                {line}
              </span>
            ))}
          </pre>
        </details>
      ))}
    </section>
  );
}

// shown drops git's header lines, which repeat the file name, and keeps the
// hunks. A file with no hunks, such as a binary or a pure rename, keeps the
// lines that say what happened to it.
function shown(lines: string[]): string[] {
  const hunk = lines.findIndex((l) => l.startsWith("@@"));
  if (hunk >= 0) return lines.slice(hunk);
  return lines.filter((l) => !l.startsWith("diff --git ") && !l.startsWith("index "));
}

function lineClass(line: string): string | undefined {
  if (line.startsWith("@@")) return "hunk";
  if (line.startsWith("+")) return "add";
  if (line.startsWith("-")) return "del";
  return undefined;
}
