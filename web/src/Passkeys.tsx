// SPDX-License-Identifier: Apache-2.0
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { passkeysWork, type PasskeyApi, type PasskeyList } from "./passkey";

// Passkeys lists your passkeys and lets you add and remove them. Once you
// have one, any change needs a fresh passkey check, which it asks for first.
export function Passkeys({ api }: { api: PasskeyApi }) {
  const [state, setState] = useState<PasskeyList | null>(null);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    try {
      setState(await api.list());
    } catch (err: unknown) {
      setError(errorText(err));
    }
  }, [api]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError(null);
    try {
      // Changing passkeys needs a recent passkey check when you have one.
      const current = await api.list();
      if (current.passkeys.length > 0 && !current.fresh) await api.verify();
      await action();
    } catch (err: unknown) {
      setError(errorText(err));
    } finally {
      setBusy(false);
      await refresh();
    }
  }

  function add(e: FormEvent) {
    e.preventDefault();
    void run(async () => {
      await api.add(name.trim());
      setName("");
    });
  }

  const here = state ? passkeysWork(state.origin) : false;

  return (
    <section aria-labelledby="passkeys-title">
      <h2 id="passkeys-title">Passkeys</h2>
      <p>
        A passkey signs you in with your fingerprint, face or device PIN. Once you add one, the token no longer signs
        in, unless <code>tess serve</code> runs with <code>--allow-token-login</code>.
      </p>
      {state === null && !error && <p>Loading...</p>}
      {state?.passkeys.length === 0 && <p>No passkeys yet.</p>}
      {state && state.passkeys.length > 0 && (
        <ul className="repos">
          {state.passkeys.map((p) => (
            <li key={p.id}>
              <strong>{p.name}</strong>{" "}
              <span className="hint">
                added {day(p.createdAt)}, {p.lastUsedAt ? `last used ${day(p.lastUsedAt)}` : "not used yet"}
              </span>{" "}
              <button
                type="button"
                disabled={busy || !here}
                onClick={() => void run(() => api.remove(p.id))}
                aria-label={`Remove ${p.name}`}
              >
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}
      {state && !here && (
        <p>
          Passkeys work only at <a href={state.origin}>{state.origin}</a>. Open it to add or remove one.
        </p>
      )}
      {here && (
        <form onSubmit={add}>
          <label>
            Passkey name
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Laptop" maxLength={64} />
          </label>
          <button type="submit" disabled={busy}>
            Add a passkey
          </button>
        </form>
      )}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}

function day(iso: string): string {
  return new Date(iso).toLocaleDateString();
}

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

// ResetNotice warns after tess auth reset ran: every passkey is gone, so
// whoever signs in next should know, and add a passkey again.
export function ResetNotice({ api }: { api: PasskeyApi }) {
  const [at, setAt] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    api
      .resetNotice()
      .then((t) => active && setAt(t))
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [api]);

  if (at === null) return null;
  return (
    <div role="alert" className="notice">
      <p>
        Sign-in was reset from the host on {new Date(at).toLocaleString()}. Every passkey was removed and everyone was
        signed out. If you did not run tess auth reset, someone with access to this machine did. Add a passkey again in
        the Passkeys tab.
      </p>
      <button type="button" onClick={() => api.dismissResetNotice().then(() => setAt(null), () => undefined)}>
        Dismiss
      </button>
    </div>
  );
}
