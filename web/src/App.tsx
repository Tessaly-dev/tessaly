// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from "react";
import { authApi, type AuthApi } from "./api";
import { fetchHealth, type Health } from "./health";
import { Repos } from "./Repos";
import { reposApi, type Repo, type ReposApi } from "./repos";
import { Home } from "./Home";
import { useWorkspaces } from "./useWorkspaces";
import { workspacesApi, type WorkspacesApi } from "./workspaces";
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
  repos?: ReposApi;
  workspaces?: WorkspacesApi;
};

const defaultAuth = authApi();
const defaultRepos = reposApi();
const defaultWorkspaces = workspacesApi();

export function App({ load = fetchHealth, auth = defaultAuth, initialToken = null, repos = defaultRepos,
  workspaces = defaultWorkspaces,
}: Props) {
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
      <header className="top">
        <img src="/favicon.svg" alt="" width={32} height={32} />
        <h1>Tessaly</h1>
        <p role="status" className="hint">
          {health.kind === "loading" && "Checking the daemon..."}
          {health.kind === "ok" && `Daemon ${health.health.status}, version ${health.health.version}`}
          {health.kind === "error" && `Daemon unreachable: ${health.message}`}
        </p>
        {authState === "signedIn" && (
          <button type="button" onClick={() => signOut().catch(() => undefined)}>
            Sign out
          </button>
        )}
      </header>
      {authState === "signedOut" && (
        <>
          <p>Run many coding agents in parallel, on your own machine.</p>
          <SignIn onSignIn={signIn} />
        </>
      )}
      {authState === "signedIn" && <SignedIn repos={repos} workspaces={workspaces} />}
    </main>
  );
}

type Tab = "home" | "repos";

const tabs: { key: Tab; label: string }[] = [
  { key: "home", label: "Home" },
  { key: "repos", label: "Repositories" },
];

// SignedIn holds the tabs. Every panel stays mounted, so a terminal keeps
// its connection and the repo list stays loaded while another tab shows.
function SignedIn({ repos, workspaces }: { repos: ReposApi; workspaces: WorkspacesApi }) {
  const [tab, setTab] = useState<Tab>("home");
  const [repoList, setRepoList] = useState<Repo[]>([]);
  const [selected, setSelected] = useState<number | null>(null);
  const work = useWorkspaces(workspaces);

  return (
    <>
      <nav role="tablist" aria-label="Views" className="tabs">
        {tabs.map((t) => (
          <button
            key={t.key}
            type="button"
            role="tab"
            id={`tab-${t.key}`}
            aria-selected={tab === t.key}
            aria-controls={`panel-${t.key}`}
            onClick={() => setTab(t.key)}
          >
            {t.label}
          </button>
        ))}
      </nav>
      {work.error && <p role="alert">{work.error}</p>}
      <div role="tabpanel" id="panel-home" aria-labelledby="tab-home" hidden={tab !== "home"}>
        <Home
          list={work.list}
          repos={repoList}
          agents={work.agents}
          api={workspaces}
          refresh={work.refresh}
          selected={selected}
          onSelect={setSelected}
        />
      </div>
      <div role="tabpanel" id="panel-repos" aria-labelledby="tab-repos" hidden={tab !== "repos"}>
        <Repos api={repos} onChange={setRepoList} />
      </div>
    </>
  );
}

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
