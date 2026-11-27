// SPDX-License-Identifier: Apache-2.0

// Passkey sign-in and passkey management. The daemon sends WebAuthn options
// as JSON with binary fields in base64url, and takes the browser's reply the
// same way. These helpers convert between that JSON and the browser API.
import { apiFetch, errorFrom, setSessionKey } from "./http";

export type LoginOptions = { token: boolean; passkey: boolean; passkeyOrigin: string };

export type Passkey = { id: number; name: string; createdAt: string; lastUsedAt: string | null };

export type PasskeyList = { passkeys: Passkey[]; fresh: boolean; origin: string };

export type PasskeyApi = {
  loginOptions(): Promise<LoginOptions>;
  signIn(): Promise<void>;
  list(): Promise<PasskeyList>;
  add(name: string): Promise<void>;
  verify(): Promise<void>;
  remove(id: number): Promise<void>;
  // resetNotice gives the time tess auth reset last ran, until dismissed.
  resetNotice(): Promise<string | null>;
  dismissResetNotice(): Promise<void>;
};

// WebAuthn is the part of navigator.credentials the passkey calls use, so
// tests can pass a fake.
export type WebAuthn = {
  create(options: CredentialCreationOptions): Promise<Credential | null>;
  get(options: CredentialRequestOptions): Promise<Credential | null>;
};

type Begun = { ceremony: string; options: { publicKey: Record<string, unknown> } };

export function passkeyApi(fetcher: typeof fetch = apiFetch, webauthn?: WebAuthn): PasskeyApi {
  const browser = (): WebAuthn => webauthn ?? navigator.credentials;

  async function post(path: string, body?: unknown): Promise<Response> {
    return fetcher(path, {
      method: "POST",
      headers: body === undefined ? {} : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  }

  async function begin(path: string, action: string): Promise<Begun> {
    const res = await post(path);
    if (!res.ok) throw await errorFrom(res, action);
    return (await res.json()) as Begun;
  }

  // assert asks the browser to sign the ceremony that path starts, and
  // returns the body to finish it with.
  async function assert(path: string, action: string) {
    const { ceremony, options } = await begin(path, action);
    const cred = await browser().get({ publicKey: requestOptions(options.publicKey) });
    if (cred === null) throw new Error("No passkey was chosen.");
    return { ceremony, credential: assertionJSON(cred as PublicKeyCredential) };
  }

  return {
    async loginOptions() {
      const res = await fetcher("/api/v1/login", { headers: { Accept: "application/json" } });
      if (!res.ok) throw await errorFrom(res, "loading sign-in options");
      return (await res.json()) as LoginOptions;
    },

    async signIn() {
      const body = await assert("/api/v1/login/passkey/begin", "starting passkey sign-in");
      const res = await post("/api/v1/login/passkey/finish", body);
      if (res.status === 429) throw new Error("Too many failed sign-ins. Try again in a minute.");
      if (!res.ok) throw await errorFrom(res, "passkey sign-in");
      const reply = (await res.json()) as { key?: unknown };
      if (typeof reply.key !== "string" || reply.key === "") throw new Error("sign-in reply has no session key");
      setSessionKey(reply.key);
    },

    async list() {
      const res = await fetcher("/api/v1/passkeys", { headers: { Accept: "application/json" } });
      if (!res.ok) throw await errorFrom(res, "listing passkeys");
      return (await res.json()) as PasskeyList;
    },

    async add(name: string) {
      const { ceremony, options } = await begin("/api/v1/passkeys/register/begin", "adding a passkey");
      const cred = await browser().create({ publicKey: creationOptions(options.publicKey) });
      if (cred === null) throw new Error("No passkey was made.");
      const res = await post("/api/v1/passkeys/register/finish", {
        ceremony,
        name,
        credential: attestationJSON(cred as PublicKeyCredential),
      });
      if (!res.ok) throw await errorFrom(res, "adding a passkey");
    },

    async verify() {
      const body = await assert("/api/v1/passkeys/verify/begin", "checking your passkey");
      const res = await post("/api/v1/passkeys/verify/finish", body);
      if (!res.ok) throw await errorFrom(res, "checking your passkey");
    },

    async remove(id: number) {
      const res = await fetcher(`/api/v1/passkeys/${encodeURIComponent(String(id))}`, { method: "DELETE" });
      if (!res.ok) throw await errorFrom(res, "removing the passkey");
    },

    async resetNotice() {
      const res = await fetcher("/api/v1/reset-notice", { headers: { Accept: "application/json" } });
      if (!res.ok) throw await errorFrom(res, "reading the reset notice");
      return ((await res.json()) as { resetAt: string | null }).resetAt;
    },

    async dismissResetNotice() {
      const res = await fetcher("/api/v1/reset-notice", { method: "DELETE" });
      if (!res.ok) throw await errorFrom(res, "dismissing the reset notice");
    },
  };
}

// passkeysWork reports whether this page can use passkeys: the browser has
// WebAuthn and the page is on the address the daemon accepts them from.
export function passkeysWork(origin: string, here: string = window.location.origin): boolean {
  return origin !== "" && here === origin && typeof PublicKeyCredential === "function";
}

export function fromBase64url(s: string): ArrayBuffer {
  const b64 = s.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - (s.length % 4)) % 4);
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out.buffer;
}

export function toBase64url(buf: ArrayBuffer): string {
  let bin = "";
  for (const b of new Uint8Array(buf)) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

type Descriptor = { id: string; type: string; transports?: string[] };

function descriptors(list: unknown): PublicKeyCredentialDescriptor[] | undefined {
  if (!Array.isArray(list)) return undefined;
  return (list as Descriptor[]).map((d) => ({
    ...d,
    type: "public-key",
    id: fromBase64url(d.id),
    transports: d.transports as AuthenticatorTransport[] | undefined,
  }));
}

export function creationOptions(o: Record<string, unknown>): PublicKeyCredentialCreationOptions {
  const user = o.user as { id: string; name: string; displayName: string };
  return {
    ...(o as unknown as PublicKeyCredentialCreationOptions),
    challenge: fromBase64url(o.challenge as string),
    user: { ...user, id: fromBase64url(user.id) },
    excludeCredentials: descriptors(o.excludeCredentials),
  };
}

export function requestOptions(o: Record<string, unknown>): PublicKeyCredentialRequestOptions {
  return {
    ...(o as unknown as PublicKeyCredentialRequestOptions),
    challenge: fromBase64url(o.challenge as string),
    allowCredentials: descriptors(o.allowCredentials),
  };
}

function base(cred: PublicKeyCredential) {
  return {
    id: cred.id,
    rawId: toBase64url(cred.rawId),
    type: cred.type,
    authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
    clientExtensionResults: cred.getClientExtensionResults(),
  };
}

export function attestationJSON(cred: PublicKeyCredential) {
  const r = cred.response as AuthenticatorAttestationResponse;
  return {
    ...base(cred),
    response: {
      clientDataJSON: toBase64url(r.clientDataJSON),
      attestationObject: toBase64url(r.attestationObject),
      transports: typeof r.getTransports === "function" ? r.getTransports() : undefined,
    },
  };
}

export function assertionJSON(cred: PublicKeyCredential) {
  const r = cred.response as AuthenticatorAssertionResponse;
  return {
    ...base(cred),
    response: {
      clientDataJSON: toBase64url(r.clientDataJSON),
      authenticatorData: toBase64url(r.authenticatorData),
      signature: toBase64url(r.signature),
      userHandle: r.userHandle ? toBase64url(r.userHandle) : undefined,
    },
  };
}
