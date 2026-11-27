// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type FormEvent } from "react";
import type { Agent, Workspace, WorkspacesApi } from "./workspaces";

type Props = {
  ws: Workspace;
  agents: Agent[];
  api: WorkspacesApi;
  onChange: () => Promise<void>;
  // pollMs sets how often output refreshes while the agent runs.
  pollMs?: number;
};

// AgentPanel starts and stops a workspace's agent and shows its output.
export function AgentPanel({ ws, agents, api, onChange, pollMs = 1000 }: Props) {
  const [agent, setAgent] = useState("");
  const [prompt, setPrompt] = useState("");
  const [output, setOutput] = useState("");
  const [error, setError] = useState<string | null>(null);
  const running = ws.state === "running";
  const chosen = agents.find((a) => a.name === agent) ?? agents[0];

  useEffect(() => {
    let active = true;
    const load = () =>
      api
        .output(ws.id)
        .then((text) => active && setOutput(text))
        .catch(() => undefined);
    void load();
    if (!running) return () => void (active = false);
    const timer = setInterval(load, pollMs);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [api, ws.id, running, pollMs]);

  async function run(action: () => Promise<unknown>) {
    setError(null);
    try {
      await action();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
    await onChange();
  }

  function start(e: FormEvent) {
    e.preventDefault();
    if (chosen) void run(() => api.start(ws.id, chosen.name, prompt));
  }

  return (
    <div className="agent">
      {running ? (
        <button type="button" onClick={() => void run(() => api.stop(ws.id))}>
          Stop agent
        </button>
      ) : agents.length === 0 ? (
        <p className="hint">
          No agents set up. Add one to <code>~/.tessaly/config.toml</code> and restart <code>tess serve</code>.
        </p>
      ) : (
        <form onSubmit={start}>
          <label>
            Agent
            <select value={chosen?.name} onChange={(e) => setAgent(e.target.value)}>
              {agents.map((a) => (
                <option key={a.name} value={a.name}>
                  {a.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Prompt
            <textarea value={prompt} onChange={(e) => setPrompt(e.target.value)} rows={3} />
          </label>
          <button type="submit">Start agent</button>
        </form>
      )}
      {ws.exitCode !== undefined && !running && <p className="hint">Last run exited with code {ws.exitCode}.</p>}
      {error && <p role="alert">{error}</p>}
      {output && (
        <pre className="output" aria-label="Agent output">
          {output}
        </pre>
      )}
    </div>
  );
}
