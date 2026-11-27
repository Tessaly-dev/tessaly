# Sign in

Every API call needs you to sign in. This keeps other people and programs on the same machine from using Tessaly as you.

There are two ways to sign in: a passkey, and the login token. You use the token the first time, then add a passkey. Once a passkey exists, the token no longer signs in.

## The login token

The first time `tess serve` starts, it writes a random token to `~/.tessaly/token`. Only your OS user can read that file (mode 0600), and the folder is mode 0700. `tess serve` refuses to start if other users can read either one.

To move the data folder, set `TESSALY_HOME` to an absolute path. The folder also holds `tessaly.db`, Tessaly's state.

### Signing in with the token

- In a terminal, `tess serve` prints a link like `http://localhost:7777/#token=...`. Open it and you are signed in. The token sits after `#`, so your browser never sends it to the server, and the page removes it from the address bar.
- When stdout is not a terminal, for example under systemd, `tess serve` prints only the path of the token file. Open the UI and paste the token into the form.

## Passkeys

A passkey signs you in with your fingerprint, face or device PIN, through WebAuthn. It cannot be phished or copied out of a log file the way a token can.

1. Sign in with the token at `http://localhost:7777`.
2. Open the Passkeys tab, give the passkey a name and select Add a passkey.
3. Next time, select Sign in with a passkey.

Passkeys work only at `http://localhost:<port>`. Browsers do not accept an IP address such as `127.0.0.1` as a passkey site, and they treat `localhost` as secure without HTTPS. An SSH tunnel to `localhost` works as is: `ssh -L 7777:127.0.0.1:7777 you@host`, then open `http://localhost:7777`. If you open Tessaly at `127.0.0.1`, the sign-in page links to the `localhost` address.

### Token sign-in turns off

Once you have a passkey, the token no longer signs in, and `tess serve` prints a passkey hint instead of the token link. Sessions that were signed in keep working.

To keep token sign-in on, for example for scripts that call the API, start `tess serve --allow-token-login`.

### Changing your passkeys

Adding a second passkey, or removing one, needs a passkey check from the last 5 minutes, even inside a signed-in session. Signing in with a passkey counts as a check. Otherwise the UI asks for your passkey first. This stops someone who only has the token from swapping in their own passkey.

Removing a passkey ends every session that signed in with it. Removing the last one turns token sign-in back on.

Each install makes its own random WebAuthn user ID, so two installs on the same port, such as one on the host and one in a container, do not replace each other's passkeys in your browser.

## Sessions

A sign-in has two parts. The browser keeps a cookie, and the page keeps a session key in its local storage. Browsers send cookies for `localhost` and `127.0.0.1` to every port, so another local server you open could get the cookie. Local storage belongs to one origin, port included, so that server never sees the key, and the cookie alone does not work.

After 10 failed sign-ins in a minute, wrong tokens and refused passkeys together, sign-in closes for the rest of that minute, even for the right token or passkey, and replies `429` with a `Retry-After` header.

A sign-in lasts 7 days. Restarting `tess serve` signs everyone out. Use the Sign out button to end a session early. It also deletes the key from the browser.

## Calling the API yourself

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

## Lost your passkeys: tess auth reset

Run this on the machine that runs `tess serve`:

```sh
tess auth reset
```

It shows this machine's host name and asks you to type it, so a reset only happens on purpose and on the machine you meant. A script can pipe the name in, for example `hostname | tess auth reset`. Then it removes every passkey, ends every session and writes a new token. The old token stops working. In a terminal it prints a new sign-in link.

The new token signs in once, within 15 minutes. If you reset while `tess serve` is stopped, the 15 minutes start when it starts. Add a passkey right after you sign in: once the token is used, it does not sign in again, and you need another reset. With `tess serve --allow-token-login` the token keeps working as before.

The command refuses to run from an agent's terminal. Agents run as your OS user, so without this check an agent could reset sign-in and read the new token. The daemon walks the caller's process tree and refuses it when it runs under an agent that Tessaly started.

- While `tess serve` runs, the command asks the daemon over its control socket, `~/.tessaly/run/control.sock`. The socket is mode 0600 in a 0700 folder, and the daemon refuses callers that run as another OS user. It is not part of the HTTP server, so no web page or network client can reach it.
- While `tess serve` is stopped, the command changes the token file and `tessaly.db` itself. The daemon holds a lock file, `~/.tessaly/run/tess.lock`, while it runs, which is how the command tells the two cases apart. The lock also stops a second `tess serve` from using the same data folder.

Each reset writes a row to the `auth_events` table in `tessaly.db`, and a running daemon logs a warning. Every session ends, so a reset you did not make shows up as being signed out. Whoever signs in next sees a notice that sign-in was reset, with the time, until they dismiss it.

In Docker, run `docker exec -it tess tess auth reset` and type the container's host name it shows. Then read the new token with `docker exec tess cat /data/token`.

## Changing the token

Run `tess auth reset`. It also removes your passkeys. To change only the token, stop `tess serve`, delete `~/.tessaly/token` and start it again.

## Keep it secret

Anyone with the token can run Tessaly as you while token sign-in is on. Do not paste it into chats or commit it.
