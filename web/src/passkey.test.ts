// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  creationOptions,
  fromBase64url,
  passkeyApi,
  passkeysWork,
  requestOptions,
  toBase64url,
  type WebAuthn,
} from "./passkey";
import { sessionKey, setSessionKey } from "./http";

afterEach(() => setSessionKey(null));

const bytes = (...b: number[]) => new Uint8Array(b).buffer;

describe("base64url", () => {
  it("round-trips bytes without padding", () => {
    const buf = bytes(0, 251, 255, 1, 2);
    const s = toBase64url(buf);
    expect(s).toBe("APv_AQI");
    expect(new Uint8Array(fromBase64url(s))).toEqual(new Uint8Array(buf));
  });
});

describe("options", () => {
  it("decodes the binary fields of creation options", () => {
    const o = creationOptions({
      challenge: "AQI",
      rp: { id: "localhost", name: "Tessaly" },
      user: { id: "AwQ", name: "tessaly", displayName: "Tessaly" },
      pubKeyCredParams: [{ type: "public-key", alg: -7 }],
      excludeCredentials: [{ type: "public-key", id: "BQY" }],
    });
    expect(new Uint8Array(o.challenge as ArrayBuffer)).toEqual(new Uint8Array([1, 2]));
    expect(new Uint8Array(o.user.id as ArrayBuffer)).toEqual(new Uint8Array([3, 4]));
    expect(new Uint8Array(o.excludeCredentials?.[0].id as ArrayBuffer)).toEqual(new Uint8Array([5, 6]));
    expect(o.rp.id).toBe("localhost");
  });

  it("decodes the binary fields of request options", () => {
    const o = requestOptions({ challenge: "AQI", allowCredentials: [{ type: "public-key", id: "BQY" }] });
    expect(new Uint8Array(o.allowCredentials?.[0].id as ArrayBuffer)).toEqual(new Uint8Array([5, 6]));
  });
});

describe("passkeysWork", () => {
  it("needs the passkey origin", () => {
    vi.stubGlobal("PublicKeyCredential", function PublicKeyCredential() {});
    expect(passkeysWork("http://localhost:7777", "http://localhost:7777")).toBe(true);
    expect(passkeysWork("http://localhost:7777", "http://127.0.0.1:7777")).toBe(false);
    expect(passkeysWork("", "http://localhost:7777")).toBe(false);
    vi.unstubAllGlobals();
    expect(passkeysWork("http://localhost:7777", "http://localhost:7777")).toBe(false);
  });
});

// fakeCredential looks like what navigator.credentials returns.
function fakeCredential(response: Record<string, ArrayBuffer | null>) {
  return {
    id: "AQ",
    rawId: bytes(1),
    type: "public-key",
    authenticatorAttachment: "platform",
    getClientExtensionResults: () => ({}),
    response: { ...response, getTransports: () => ["internal"] },
  } as unknown as Credential;
}

// server answers begin calls with a ceremony and records every call.
function server(finish: Response = Response.json({ key: "k1" })) {
  const calls: { url: string; body: unknown }[] = [];
  const fetcher = vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ url, body: init?.body ? JSON.parse(init.body as string) : undefined });
    if (url.endsWith("/begin")) {
      return Response.json({
        ceremony: "c1",
        options: {
          publicKey: { challenge: "AQI", user: { id: "AwQ", name: "t", displayName: "T" }, allowCredentials: [] },
        },
      });
    }
    return finish;
  }) as unknown as typeof fetch;
  return { calls, fetcher };
}

describe("passkeyApi", () => {
  it("signs in with a passkey and keeps the session key", async () => {
    const { calls, fetcher } = server();
    const webauthn: WebAuthn = {
      create: vi.fn(),
      get: vi.fn(async () =>
        fakeCredential({
          clientDataJSON: bytes(1),
          authenticatorData: bytes(2),
          signature: bytes(3),
          userHandle: null,
        }),
      ),
    };
    await passkeyApi(fetcher, webauthn).signIn();
    expect(calls.map((c) => c.url)).toEqual(["/api/v1/login/passkey/begin", "/api/v1/login/passkey/finish"]);
    expect(calls[1].body).toMatchObject({
      ceremony: "c1",
      credential: { rawId: "AQ", response: { clientDataJSON: "AQ", authenticatorData: "Ag", signature: "Aw" } },
    });
    expect(sessionKey()).toBe("k1");
  });

  it("explains a refused passkey and too many failures", async () => {
    const webauthn: WebAuthn = {
      create: vi.fn(),
      get: vi.fn(async () =>
        fakeCredential({ clientDataJSON: bytes(1), authenticatorData: bytes(2), signature: bytes(3), userHandle: null }),
      ),
    };
    const refused = server(Response.json({ error: "that passkey did not sign in" }, { status: 401 }));
    await expect(passkeyApi(refused.fetcher, webauthn).signIn()).rejects.toThrow("did not sign in");
    const limited = server(new Response(null, { status: 429 }));
    await expect(passkeyApi(limited.fetcher, webauthn).signIn()).rejects.toThrow("Too many");
    expect(sessionKey()).toBeNull();
  });

  it("stops when no passkey is chosen", async () => {
    const { fetcher, calls } = server();
    const webauthn: WebAuthn = { create: vi.fn(), get: vi.fn(async () => null) };
    await expect(passkeyApi(fetcher, webauthn).signIn()).rejects.toThrow("No passkey");
    expect(calls).toHaveLength(1);
  });

  it("adds a passkey with its name", async () => {
    const { calls, fetcher } = server(new Response(null, { status: 201 }));
    const webauthn: WebAuthn = {
      create: vi.fn(async () => fakeCredential({ clientDataJSON: bytes(1), attestationObject: bytes(4) })),
      get: vi.fn(),
    };
    await passkeyApi(fetcher, webauthn).add("Laptop");
    expect(calls[1]).toMatchObject({
      url: "/api/v1/passkeys/register/finish",
      body: {
        ceremony: "c1",
        name: "Laptop",
        credential: { response: { attestationObject: "BA", transports: ["internal"] } },
      },
    });
  });
});
