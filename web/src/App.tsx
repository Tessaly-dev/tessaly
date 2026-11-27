// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from "react";
import { fetchHealth, type Health } from "./health";

type State =
  | { kind: "loading" }
  | { kind: "ok"; health: Health }
  | { kind: "error"; message: string };

export function App({ load = fetchHealth }: { load?: () => Promise<Health> }) {
  const [state, setState] = useState<State>({ kind: "loading" });

  useEffect(() => {
    let active = true;
    load()
      .then((health) => active && setState({ kind: "ok", health }))
      .catch((err: unknown) =>
        active && setState({ kind: "error", message: err instanceof Error ? err.message : String(err) }),
      );
    return () => {
      active = false;
    };
  }, [load]);

  return (
    <main>
      <img src="/favicon.svg" alt="" width={48} height={48} />
      <h1>Tessaly</h1>
      <p>Run many coding agents in parallel, on your own machine.</p>
      <p role="status">
        {state.kind === "loading" && "Checking the daemon..."}
        {state.kind === "ok" && `Daemon ${state.health.status}, version ${state.health.version}`}
        {state.kind === "error" && `Daemon unreachable: ${state.message}`}
      </p>
    </main>
  );
}
