// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent, type ReactNode } from "react";
import { Terminal } from "./Terminal";
import type { Agent, Workspace, WorkspacesApi } from "./workspaces";

type Props = {
  ws: Workspace;
  agents: Agent[];
  api: WorkspacesApi;
  onChange: () => Promise<void>;
  // terminal renders the live terminal. Tests pass a stand-in.
  terminal?: (props: TerminalProps) => ReactNode;
};

type TerminalProps = { workspaceId: number; session: string; onClose: () => void };

// AgentPanel starts and stops a workspace's agent and shows its terminal.
export function AgentPanel({ ws, agents, api, onChange, terminal = (p) => <Terminal {...p} /> }: Props) {
  const [agent, setAgent] = useState("");
  const [prompt, setPrompt] = useState("");
  const [error, setError] = useState<string | null>(null);
  const running = ws.state === "running";
  const chosen = agents.find((a) => a.name === agent) ?? agents[0];

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
      {(running || ws.agent) &&
        // The session key changes when the agent starts or stops, so the
        // terminal reconnects and shows the new run or the transcript.
        terminal({ workspaceId: ws.id, session: `${ws.state}-${ws.agent ?? ""}`, onClose: () => void onChange() })}
    </div>
  );
}
