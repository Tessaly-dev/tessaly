// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from "react";
import type { Repo } from "./repos";
import { slugify, type Workspace, type WorkspacesApi } from "./workspaces";

type Props = {
  repos: Repo[];
  api: WorkspacesApi;
  onCreated: (ws: Workspace) => void;
};

// NewWorkspace creates a workspace in a chosen repo.
export function NewWorkspace({ repos, api, onCreated }: Props) {
  const [repoId, setRepoId] = useState<number | null>(null);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);

  if (repos.length === 0) return <p className="hint">Add a repository to start a workspace.</p>;
  // Pick the first repo until the user picks one, and drop a removed one.
  const selected = repos.find((r) => r.id === repoId) ?? repos[0];

  async function create(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const ws = await api.create(selected.id, slugify(name));
      setName("");
      onCreated(ws);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <form onSubmit={(e) => void create(e)} className="new-ws">
      <label>
        Repository
        <select value={selected.id} onChange={(e) => setRepoId(Number(e.target.value))}>
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
      {error && <p role="alert">{error}</p>}
    </form>
  );
}
