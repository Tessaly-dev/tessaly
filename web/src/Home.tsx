// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { AgentPanel } from "./AgentPanel";
import { groupForHome, reason } from "./attention";
import { NewWorkspace } from "./NewWorkspace";
import type { Repo } from "./repos";
import type { Agent, Workspace, WorkspacesApi } from "./workspaces";

type Props = {
  list: Workspace[] | null;
  repos: Repo[];
  agents: Agent[];
  api: WorkspacesApi;
  refresh: () => Promise<void>;
  selected: number | null;
  onSelect: (id: number | null) => void;
};

// Home lists work by what needs you: agents waiting for input, finished work
// to review, failed runs, then running agents. The chosen workspace opens
// beside the list with its live terminal.
export function Home({ list, repos, agents, api, refresh, selected, onSelect }: Props) {
  const [error, setError] = useState<string | null>(null);
  const [showNew, setShowNew] = useState(false);
  const groups = groupForHome(list ?? []);
  const repoName = (id: number) => repos.find((r) => r.id === id)?.name ?? `repo ${id}`;
  const open = list?.find((w) => w.id === selected && w.state !== "archived");

  // canResume reports whether the agent of ws's last run has a resume command.
  const canResume = (ws: Workspace) => (agents.find((a) => a.name === ws.agent)?.resume?.length ?? 0) > 0;

  async function resume(ws: Workspace) {
    setError(null);
    try {
      await api.resume(ws.id);
      onSelect(ws.id);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
    await refresh();
  }

  async function archive(ws: Workspace) {
    setError(null);
    try {
      await api.archive(ws.id);
      onSelect(null);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
    await refresh();
  }

  return (
    <div className="home">
      <section aria-labelledby="needs-title" className="needs">
        <h2 id="needs-title">Your work</h2>
        {error && !open && <p role="alert">{error}</p>}
        {list && groups.length === 0 && <p className="hint">Nothing here yet. Create a workspace below.</p>}
        {groups.map((g) => (
          <section key={g.key} aria-label={g.title} className={`group group-${g.key}`}>
            <h3>
              {g.title} <span className="count">{g.items.length}</span>
            </h3>
            <ul>
              {g.items.map((w) => (
                <li key={w.id}>
                  <button
                    type="button"
                    className="item"
                    aria-current={w.id === open?.id}
                    onClick={() => onSelect(w.id)}
                  >
                    <strong>{w.slug}</strong>
                    <span className="item-meta">
                      {repoName(w.repoId)}
                      {w.agent && ` · ${w.agent}`}
                    </span>
                    <span className="item-reason">{reason(w)}</span>
                  </button>
                  {w.ended === "interrupted" && w.state === "exited" && canResume(w) && (
                    <button type="button" className="resume" onClick={() => void resume(w)} aria-label={`Resume ${w.slug}`}>
                      Resume
                    </button>
                  )}
                </li>
              ))}
            </ul>
          </section>
        ))}
        <div className="new">
          {list?.length !== 0 && (
            <button type="button" aria-expanded={showNew} onClick={() => setShowNew(!showNew)}>
              New workspace
            </button>
          )}
          {(showNew || list?.length === 0) && (
            <NewWorkspace
              repos={repos}
              api={api}
              onCreated={(ws) => {
                setShowNew(false);
                onSelect(ws.id);
                void refresh();
              }}
            />
          )}
        </div>
      </section>

      <section aria-label="Workspace" className="detail">
        {open ? (
          <>
            <header>
              <h2>{open.slug}</h2>
              <p className="hint">
                {repoName(open.repoId)} · <code>{open.branch}</code> · <span className="state">{open.state}</span>
              </p>
              <p className="item-reason">{reason(open)}</p>
              {open.state !== "running" && open.state !== "creating" && (
                <button type="button" onClick={() => void archive(open)} aria-label={`Archive ${open.slug}`}>
                  Archive
                </button>
              )}
              {error && <p role="alert">{error}</p>}
            </header>
            {open.state !== "creating" && open.state !== "failed" && (
              // key resets the start form when you switch workspaces.
              <AgentPanel key={open.id} ws={open} agents={agents} api={api} onChange={refresh} />
            )}
          </>
        ) : (
          <p className="hint">Pick a workspace to see its agent.</p>
        )}
      </section>
    </div>
  );
}
