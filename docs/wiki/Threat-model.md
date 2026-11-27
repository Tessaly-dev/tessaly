# Threat model

Tessaly runs with your user's rights, so treat anyone who reaches its API as able to act as you. `tess serve` binds to `127.0.0.1` by default, needs a session on every API request, checks the `Host` header of every request against `localhost`, `127.0.0.1` and your `public_url`, and sets strict browser security headers.

The controls in the code, and the test for each one, are listed in [docs/threat-model.md](https://github.com/tessaly-dev/tessaly/blob/main/docs/threat-model.md).

Found a vulnerability? Do not open an issue. Email security@tessaly.dev.
