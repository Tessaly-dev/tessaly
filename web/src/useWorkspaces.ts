// SPDX-License-Identifier: Apache-2.0
import { useCallback, useEffect, useState } from "react";
import type { Agent, Workspace, WorkspacesApi } from "./workspaces";

// pollMs is how often the workspace list reloads, so agents that start
// waiting or finish move between groups without a page reload.
export const pollMs = 2000;

// useWorkspaces loads the workspace list and the agents, and reloads the
// list every pollMs.
export function useWorkspaces(api: WorkspacesApi) {
  const [list, setList] = useState<Workspace[] | null>(null);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setList(await api.list());
      setError(null);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [api]);

  useEffect(() => {
    void refresh();
    api
      .agents()
      .then(setAgents)
      .catch(() => setAgents([]));
    const timer = setInterval(() => void refresh(), pollMs);
    return () => clearInterval(timer);
  }, [api, refresh]);

  return { list, agents, error, refresh };
}
