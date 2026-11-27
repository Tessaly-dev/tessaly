// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from "react";
import { passkeysWork, type LoginOptions } from "./passkey";

type Props = {
  // options says which ways to sign in are open. null while it loads or
  // when it failed, and then the token form shows.
  options: LoginOptions | null;
  onSignIn: (token: string) => Promise<void>;
  onPasskey: () => Promise<void>;
};

export function SignIn({ options, onSignIn, onPasskey }: Props) {
  const [token, setToken] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const showToken = options?.token ?? true;
  const passkey = options?.passkey ?? false;
  const passkeyHere = passkey && passkeysWork(options?.passkeyOrigin ?? "");

  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError(null);
    try {
      await action();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  function submit(e: FormEvent) {
    e.preventDefault();
    void run(() => onSignIn(token.trim()));
  }

  return (
    <section aria-labelledby="sign-in-title">
      <h2 id="sign-in-title">Sign in</h2>
      {passkeyHere && (
        <button type="button" disabled={busy} onClick={() => void run(onPasskey)}>
          Sign in with a passkey
        </button>
      )}
      {passkey && !passkeyHere && options && (
        <p>
          Passkeys work only at <a href={options.passkeyOrigin}>{options.passkeyOrigin}</a>. Open it to sign in with
          your passkey.
        </p>
      )}
      {showToken && (
        <form onSubmit={submit}>
          <p>
            Paste the token from <code>~/.tessaly/token</code> on the machine that runs <code>tess serve</code>.
          </p>
          <label>
            Token
            <input
              type="password"
              autoComplete="current-password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              required
            />
          </label>
          <button type="submit" disabled={busy}>
            Sign in
          </button>
        </form>
      )}
      {!showToken && (
        <p className="hint">
          Token sign-in is off. Lost your passkey, or already used the token from a reset? Run{" "}
          <code>tess auth reset</code> on the host.
        </p>
      )}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}
