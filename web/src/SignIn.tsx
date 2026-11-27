// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from "react";

export function SignIn({ onSignIn }: { onSignIn: (token: string) => Promise<void> }) {
  const [token, setToken] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await onSignIn(token.trim());
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit}>
      <h2>Sign in</h2>
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
      {error && <p role="alert">{error}</p>}
    </form>
  );
}
