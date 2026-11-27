// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from "react";
import { prLabel, ship as shipApi, titleFromSlug } from "./ship";
import type { Workspace } from "./workspaces";

type Props = {
  ws: Workspace;
  onChange: () => Promise<void>;
  ship?: (id: number, title: string, body: string) => Promise<Workspace>;
};

// Ship opens a pull request from the workspace's branch. Once one is open,
// it commits and pushes new changes to it.
export function Ship({ ws, onChange, ship = shipApi }: Props) {
  const [title, setTitle] = useState(() => titleFromSlug(ws.slug));
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const open = Boolean(ws.prUrl);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setDone(null);
    try {
      await ship(ws.id, title, body);
      setDone(open ? "Pushed to the pull request." : "Pull request opened.");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
    setBusy(false);
    await onChange();
  }

  if (ws.state === "running") {
    return (
      <section aria-label="Pull request" className="ship">
        {ws.prUrl && <PRLink url={ws.prUrl} />}
        <p className="hint">Stop the agent to ship its changes.</p>
      </section>
    );
  }

  return (
    <section aria-label="Pull request" className="ship">
      {ws.prUrl && <PRLink url={ws.prUrl} />}
      <form onSubmit={submit}>
        <label>
          {open ? "Commit message" : "Title"}
          <input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={256} required />
        </label>
        {!open && (
          <label>
            Description
            <textarea value={body} onChange={(e) => setBody(e.target.value)} rows={4} />
          </label>
        )}
        <p className="hint">
          {open
            ? "Commits any changes and pushes them to the pull request."
            : `Commits any changes, pushes ${ws.branch} and opens a pull request with gh.`}
        </p>
        <button type="submit" disabled={busy}>
          {busy ? "Shipping..." : open ? "Push changes" : "Open pull request"}
        </button>
      </form>
      {done && <p role="status">{done}</p>}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}

function PRLink({ url }: { url: string }) {
  return (
    <p>
      Pull request{" "}
      <a href={url} target="_blank" rel="noopener noreferrer">
        {prLabel(url)}
      </a>
    </p>
  );
}
