# Sign in

How sign-in works inside Tessaly. For how to sign in, add a passkey or reset sign-in, see [Sign in](https://tessaly.dev/docs/sign-in/) in the user docs.

Every API call needs a session. This keeps other people and programs on the same machine from using Tessaly as you. There are two ways to get one: the login token, and a passkey. Once a passkey exists, the token no longer signs in unless `tess serve` runs with `--allow-token-login`.

## The login token

The first time `tess serve` starts, it writes a random token to `<home>/token`, where `<home>` is `~/.tessaly` or `TESSALY_HOME`. The file is mode 0600 and the folder 0700. `tess serve` refuses to start if other users can reach either one. The folder also holds `tessaly.db`.

In a terminal, `tess serve` prints a link with the token after `#`, so the browser never sends it to the server. The page reads it, signs in and removes it from the address bar. When stdout is not a terminal, it prints only the path of the token file. Once a passkey exists, it prints a passkey hint instead.

## Passkeys

Passkeys use WebAuthn through `go-webauthn`. The relying party ID is `localhost`, plus the host of an `https://` `public_url` as a second site. Browsers do not accept an IP address as a relying party ID, and treat `localhost` as secure without HTTPS. Each passkey is bound to the site it was made on. On `127.0.0.1` or an `http://` `public_url`, the sign-in page links to the `localhost` address.

Adding a second passkey, or removing one, needs a passkey check from the last 5 minutes (`auth.RequireFresh`), even inside a signed-in session. Signing in with a passkey counts. This stops someone who only has the token from swapping in their own passkey.

Removing a passkey ends every session that signed in with it. Removing the last one turns token sign-in back on.

Each install makes its own random WebAuthn user ID, so two installs on the same port, such as one on the host and one in a container, do not replace each other's passkeys in the browser.

## Sessions

A session has two parts. The browser keeps a cookie, and the page keeps a session key in its local storage. Browsers send cookies for `localhost` and `127.0.0.1` to every port, so another local server could get the cookie. Local storage belongs to one origin, port included, so that server never sees the key, and the cookie alone does not work. The cookie is `Secure` on an `https://` `public_url`.

After 10 failed sign-ins in a minute, wrong tokens and refused passkeys together, sign-in closes for the rest of that minute, even for the right token or passkey, and replies `429` with a `Retry-After` header.

A session lasts 7 days. Sessions live in memory, so restarting `tess serve` ends them all. Sign out also deletes the key from the browser.

## API

`POST /api/v1/login` with `{"token": "..."}` replies `200` with `{"key": "..."}` and sets the `tessaly_session` cookie. It replies `403` when a passkey exists and `--allow-token-login` is off. Send both halves on every other call: the cookie, and the key in the `X-Tessaly-Key` header. A WebSocket cannot carry that header, so offer the subprotocols `tessaly` and `tessaly.key.<key>`. The server picks `tessaly`.

`GET /api/v1/login` tells you which ways to sign in are open: `{"token": true, "passkey": false, "passkeyOrigin": "http://localhost:7777"}`.

The passkey routes run a WebAuthn ceremony in two steps. A `begin` route replies `{"ceremony": "...", "options": {...}}`, where `options` goes to `navigator.credentials.create` or `get`. The matching `finish` route takes `{"ceremony": "...", "credential": {...}}` with the browser's reply, binary fields in base64url. A ceremony works once, for 2 minutes.

| Route | Session | What it does |
| --- | --- | --- |
| `POST /api/v1/login/passkey/begin`, `/finish` | No | Sign in with a passkey |
| `GET /api/v1/passkeys` | Yes | List passkeys, and whether your last passkey check is fresh |
| `POST /api/v1/passkeys/register/begin`, `/finish` | Yes | Add a passkey. `finish` also takes `"name"`. A second passkey needs a fresh check |
| `POST /api/v1/passkeys/verify/begin`, `/finish` | Yes | Check a passkey, which counts as fresh for 5 minutes |
| `DELETE /api/v1/passkeys/{id}` | Yes, fresh | Remove a passkey and end its sessions |
| `GET /api/v1/reset-notice` | Yes | `{"resetAt": "<time>"}` after `tess auth reset`, or `null` |
| `DELETE /api/v1/reset-notice` | Yes | Dismiss the reset notice |

## tess auth reset

`tess auth reset` removes every passkey, ends every session and writes a new token. It has no HTTP route, so it only works on the host. It asks the user to type the host name, which a script can pipe in.

The new token signs in once, within 15 minutes. The `token_state` setting tracks it: empty for a normal token, `reset:<time>` for an unused reset token, `reset:pending` when the reset happened while `tess serve` was stopped (the 15 minutes start on the next start), and `used` once it has signed in. With `--allow-token-login` the token keeps working as before.

The command refuses to run from an agent's terminal. Agents run as the same OS user, so without this check an agent could reset sign-in and read the new token. The daemon walks the caller's process tree (`control.InTree`) and refuses it when it runs under an agent that Tessaly started.

- While `tess serve` runs, the command asks the daemon over its control socket, `<home>/run/control.sock`. The socket is mode 0600 in a 0700 folder, and the daemon checks the caller's UID and PID and refuses other OS users. It is not part of the HTTP server, so no web page or network client can reach it.
- While `tess serve` is stopped, the command changes the token file and `tessaly.db` itself. The daemon holds a lock file, `<home>/run/tess.lock`, while it runs, which is how the command tells the two cases apart. The lock also stops a second `tess serve` from using the same data folder.

Each reset writes a row to the `auth_events` table, and a running daemon logs a warning. Whoever signs in next sees a notice with the time of the reset until they dismiss it.
