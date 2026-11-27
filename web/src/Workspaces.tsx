// SPDX-License-Identifier: Apache-2.0
import { useCallback, useEffect, useState, type FormEvent } from "react";
import type { Repo } from "./repos";
import { AgentPanel } from "./AgentPanel";
import { slugify, workspacesApi, type Agent, type Workspace, type WorkspacesApi } from "./workspaces";

const defaultApi = workspacesApi();

export function Workspaces({ repos, api = defaultApi }: { repos: Repo[]; api?: WorkspacesApi }) {
  const [list, setList] = useState<Workspace[] | null>(null);
  const [repoId, setRepoId] = useState<number | null>(null);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [open, setOpen] = useState<number | null>(null);

  const refresh = useCallback(async () => {
    try {
      setList(await api.list());
    } catch (err: unknown) {
      setError(errorText(err));
    }
  }, [api]);

  useEffect(() => {
    void refresh();
    api
      .agents()
      .then(setAgents)
      .catch(() => setAgents([]));
  }, [refresh, api]);

  // Pick the first repo until the user picks one, and drop a removed one.
  const selected = repos.find((r) => r.id === repoId) ?? repos[0];
  const repoName = (id: number) => repos.find((r) => r.id === id)?.name ?? `repo ${id}`;

  async function run(action: () => Promise<unknown>) {
    setError(null);
    try {
      await action();
    } catch (err: unknown) {
      setError(errorText(err));
    }
    await refresh();
  }

  function create(e: FormEvent) {
    e.preventDefault();
    if (!selected) return;
    void run(async () => {
      await api.create(selected.id, slugify(name));
      setName("");
    });
  }

  return (
    <section aria-labelledby="ws-title">
      <h2 id="ws-title">Workspaces</h2>
      {list?.length === 0 && <p>No workspaces yet.</p>}
      {list && list.length > 0 && (
        <ul className="repos">
          {list.map((w) => (
            <li key={w.id}>
              <strong>{w.slug}</strong> in {repoName(w.repoId)} <span className="state">{w.state}</span>{" "}
              <code>{w.branch}</code>
              {w.error && <span className="ws-error"> {w.error}</span>}{" "}
              {w.state !== "archived" && w.state !== "running" && (
                <button type="button" onClick={() => void run(() => api.archive(w.id))} aria-label={`Archive ${w.slug}`}>
                  Archive
                </button>
              )}{" "}
              {["ready", "running", "exited"].includes(w.state) && (
                <button
                  type="button"
                  onClick={() => setOpen(open === w.id ? null : w.id)}
                  aria-expanded={open === w.id}
                  aria-label={`Agent for ${w.slug}`}
                >
                  Agent
                </button>
              )}
              {open === w.id && <AgentPanel ws={w} agents={agents} api={api} onChange={refresh} />}
            </li>
          ))}
        </ul>
      )}
      {repos.length === 0 ? (
        <p>Add a repository to start a workspace.</p>
      ) : (
        <form onSubmit={create}>
          <label>
            Repository
            <select value={selected?.id} onChange={(e) => setRepoId(Number(e.target.value))}>
              {repos.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Workspace name
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="fix-login" required />
          </label>
          {name && <p className="hint">Branch: tess/{slugify(name) || "?"}</p>}
          <button type="submit">Create workspace</button>
        </form>
      )}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
