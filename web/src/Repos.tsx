// SPDX-License-Identifier: Apache-2.0
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { reposApi, type Repo, type ReposApi } from "./repos";

const defaultApi = reposApi();

type Props = {
  api?: ReposApi;
  // onChange gets the list each time it loads, so other parts can use it.
  onChange?: (repos: Repo[]) => void;
};

export function Repos({ api = defaultApi, onChange }: Props) {
  const [repos, setRepos] = useState<Repo[] | null>(null);
  const [path, setPath] = useState("");
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const list = await api.list();
      setRepos(list);
      onChange?.(list);
    } catch (err: unknown) {
      setError(errorText(err));
    }
  }, [api, onChange]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function run(action: () => Promise<unknown>) {
    setError(null);
    try {
      await action();
      await refresh();
    } catch (err: unknown) {
      setError(errorText(err));
    }
  }

  function add(e: FormEvent) {
    e.preventDefault();
    void run(async () => {
      await api.add(path.trim());
      setPath("");
    });
  }

  return (
    <section aria-labelledby="repos-title">
      <h2 id="repos-title">Repositories</h2>
      {repos === null && !error && <p>Loading...</p>}
      {repos?.length === 0 && <p>No repositories yet. Add one by its folder path.</p>}
      {repos && repos.length > 0 && (
        <ul className="repos">
          {repos.map((r) => (
            <li key={r.id}>
              <strong>{r.name}</strong> <code>{r.path}</code>{" "}
              <button type="button" onClick={() => void run(() => api.remove(r.id))} aria-label={`Remove ${r.name}`}>
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}
      <form onSubmit={add}>
        <label>
          Repository folder
          <input
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="/home/you/code/project"
            required
            spellCheck={false}
          />
        </label>
        <button type="submit">Add repository</button>
      </form>
      {error && <p role="alert">{error}</p>}
    </section>
  );
}

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
