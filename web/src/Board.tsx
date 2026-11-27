// SPDX-License-Identifier: Apache-2.0
import { groupForBoard, reason } from "./attention";
import type { Repo } from "./repos";
import type { Workspace } from "./workspaces";

type Props = {
  list: Workspace[] | null;
  repos: Repo[];
  // onOpen shows the workspace on the Home tab.
  onOpen: (id: number) => void;
};

// Board shows every workspace as a card in the column of its stage, for an
// overview of all tasks at once.
export function Board({ list, repos, onOpen }: Props) {
  const repoName = (id: number) => repos.find((r) => r.id === id)?.name ?? `repo ${id}`;
  return (
    <div className="board">
      {groupForBoard(list ?? []).map((c) => (
        <section key={c.key} aria-label={c.title} className={`column group-${c.key}`}>
          <h3>
            {c.title} <span className="count">{c.items.length}</span>
          </h3>
          <ul>
            {c.items.map((w) => (
              <li key={w.id}>
                {w.state === "archived" ? (
                  <div className="item">
                    <strong>{w.slug}</strong>
                    <span className="item-meta">
                      {repoName(w.repoId)} · <code>{w.branch}</code>
                    </span>
                  </div>
                ) : (
                  <button type="button" className="item" onClick={() => onOpen(w.id)}>
                    <strong>{w.slug}</strong>
                    <span className="item-meta">
                      {repoName(w.repoId)}
                      {w.agent && ` · ${w.agent}`}
                    </span>
                    <span className="item-reason">{reason(w)}</span>
                  </button>
                )}
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}
