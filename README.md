<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/tessaly-logo-dark.svg">
    <img alt="Tessaly logo: four agents fitting together into a T" src="docs/assets/tessaly-logo-light.svg" width="160">
  </picture>
</p>

<h1 align="center">Tessaly</h1>

<p align="center">Run many coding agents in parallel, on your own machine.</p>

<p align="center">
  <a href="https://github.com/tessaly-dev/tessaly/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/tessaly-dev/tessaly/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: Apache-2.0" src="https://img.shields.io/badge/license-Apache--2.0-B4530A"></a>
</p>

Tessaly runs coding agents side by side on your own machine. It is one Go binary with a browser UI, and it runs on your computer or your own server.

> [!NOTE]
> Tessaly is in early development and has no release yet, so build it from source. Today you can add repositories, make a branch and worktree for each task, run an agent CLI in it with a live terminal, see what needs you on the Home tab or the Board, review the diff, open a GitHub pull request and follow its checks. You sign in with a passkey, with the login token as a fallback. Read the user docs at [tessaly.dev/docs](https://tessaly.dev/docs/). They grow as features land.

## Platforms

| Platform | Support |
| --- | --- |
| macOS (Apple silicon and Intel) | Yes |
| Linux amd64 | Yes |
| Linux arm64, including Raspberry Pi | Yes |
| Windows | Run Tessaly in WSL2 and use the browser UI from Windows. There is no native Windows build. |

## Build from source

You need Go 1.27 or newer, Node 20 or newer, and pnpm.

```sh
git clone https://github.com/tessaly-dev/tessaly.git
cd tessaly
make setup   # install UI dependencies from the lockfile
make build   # build the UI and bin/tess
bin/tess version
bin/tess serve   # http://localhost:7777
```

On first start, `tess serve` makes a random login token in `~/.tessaly/token` (mode 0600). In a terminal it prints a sign-in link that carries the token. Otherwise, paste the token into the sign-in form. Then add a passkey on the Passkeys tab. Once you have one, only the passkey signs in. Lost it? Run `tess auth reset` on the host. Set `TESSALY_HOME` to keep the data folder somewhere else. See [Sign in](https://tessaly.dev/docs/sign-in/).

Check the daemon:

```sh
curl http://127.0.0.1:7777/healthz
# {"status":"ok","version":"..."}
```

`tess serve` listens on `127.0.0.1`. To reach it from another device, use an SSH tunnel, or set `public_url` in `~/.tessaly/config.toml` for an HTTPS reverse proxy or a VPN name. See [Remote access](https://tessaly.dev/docs/remote-access/). Keep the token secret: anyone who has it can use Tessaly as you.

## Security

Tessaly runs with your user's rights, so security comes first in every change. Report vulnerabilities privately to security@tessaly.dev. Read [SECURITY.md](SECURITY.md) before you report.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) for the dev setup, testing rules and the security checklist. Developer docs live in [docs/](docs/), and the developer wiki pages in [docs/wiki/](docs/wiki/). User docs live in [tessaly-dev/tessaly.dev](https://github.com/tessaly-dev/tessaly.dev). Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md).

## License

Tessaly is licensed under the [Apache License 2.0](LICENSE). Maintained by 86-88 Solutions. Website: [tessaly.dev](https://tessaly.dev). Questions: hello@tessaly.dev.
