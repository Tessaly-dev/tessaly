// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from "react";
import { authApi, type AuthApi } from "./api";
import { fetchHealth, type Health } from "./health";
import { SignIn } from "./SignIn";

type HealthState =
  | { kind: "loading" }
  | { kind: "ok"; health: Health }
  | { kind: "error"; message: string };

type AuthState = "checking" | "signedOut" | "signedIn";

type Props = {
  load?: () => Promise<Health>;
  auth?: AuthApi;
  // A token from the sign-in link, read once before the first render.
  initialToken?: string | null;
};

const defaultAuth = authApi();

export function App({ load = fetchHealth, auth = defaultAuth, initialToken = null }: Props) {
  const [health, setHealth] = useState<HealthState>({ kind: "loading" });
  const [authState, setAuthState] = useState<AuthState>("checking");

  useEffect(() => {
    let active = true;
    load()
      .then((h) => active && setHealth({ kind: "ok", health: h }))
      .catch((err: unknown) => active && setHealth({ kind: "error", message: errorText(err) }));
    return () => {
      active = false;
    };
  }, [load]);

  useEffect(() => {
    let active = true;
    // A link from "tess serve" carries the token. Sign in with it, then check
    // the session either way.
    (initialToken ? auth.signIn(initialToken).catch(() => undefined) : Promise.resolve())
      .then(() => auth.isSignedIn())
      .then((ok) => active && setAuthState(ok ? "signedIn" : "signedOut"))
      .catch(() => active && setAuthState("signedOut"));
    return () => {
      active = false;
    };
  }, [auth, initialToken]);

  async function signIn(token: string) {
    await auth.signIn(token);
    setAuthState("signedIn");
  }

  async function signOut() {
    await auth.signOut();
    setAuthState("signedOut");
  }

  return (
    <main>
      <img src="/favicon.svg" alt="" width={48} height={48} />
      <h1>Tessaly</h1>
      <p>Run many coding agents in parallel, on your own machine.</p>
      <p role="status">
        {health.kind === "loading" && "Checking the daemon..."}
        {health.kind === "ok" && `Daemon ${health.health.status}, version ${health.health.version}`}
        {health.kind === "error" && `Daemon unreachable: ${health.message}`}
      </p>
      {authState === "signedOut" && <SignIn onSignIn={signIn} />}
      {authState === "signedIn" && (
        <button type="button" onClick={() => signOut().catch(() => undefined)}>
          Sign out
        </button>
      )}
    </main>
  );
}

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
