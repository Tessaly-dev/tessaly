# Sign in

Every API call needs you to sign in. This keeps other people and programs on the same machine from using Tessaly as you.

## The login token

The first time `tess serve` starts, it writes a random token to `~/.tessaly/token`. Only your OS user can read that file (mode 0600), and the folder is mode 0700. `tess serve` refuses to start if other users can read either one.

To move the data folder, set `TESSALY_HOME` to an absolute path. The folder also holds `tessaly.db`, Tessaly's state.

## Signing in

- In a terminal, `tess serve` prints a link like `http://127.0.0.1:7777/#token=...`. Open it and you are signed in. The token sits after `#`, so your browser never sends it to the server, and the page removes it from the address bar.
- When stdout is not a terminal, for example under systemd, `tess serve` prints only the path of the token file. Open the UI and paste the token into the form.

A sign-in lasts 7 days. Restarting `tess serve` signs everyone out. Use the Sign out button to end a session early.

## Changing the token

Stop `tess serve`, delete `~/.tessaly/token` and start it again. It makes a new token, and the old one stops working.

## Keep it secret

Anyone with the token can run Tessaly as you. Do not paste it into chats or commit it.
