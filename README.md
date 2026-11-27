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
> Tessaly is in early development. Today the binary serves a health check, token sign-in, and a placeholder UI where you add and remove repositories. This README and the docs grow as features land.

## Platforms

| Platform | Support |
| --- | --- |
| macOS (Apple silicon and Intel) | Yes |
| Linux amd64 | Yes |
| Linux arm64, including Raspberry Pi | Yes |
| Windows | Run Tessaly in WSL2 and use the browser UI from Windows. There is no native Windows build. |

## Try the skeleton

You need Go 1.27 or newer, Node 20 or newer, and pnpm.

```sh
git clone https://github.com/tessaly-dev/tessaly.git
cd tessaly
make setup   # install UI dependencies from the lockfile
make build   # build the UI and bin/tess
bin/tess version
bin/tess serve   # http://127.0.0.1:7777
```

On first start, `tess serve` makes a random login token in `~/.tessaly/token` (mode 0600). In a terminal it prints a sign-in link that carries the token. Otherwise, paste the token into the sign-in form. Set `TESSALY_HOME` to keep the data folder somewhere else.

Check the daemon:

```sh
curl http://127.0.0.1:7777/healthz
# {"status":"ok","version":"..."}
```

`tess serve` only ever listens on `127.0.0.1`. To reach it from another device, use an SSH tunnel, a VPN or an HTTPS reverse proxy. Keep the token secret: anyone who has it can use Tessaly as you.

## Security

Tessaly runs with your user's rights, so security comes first in every change. Report vulnerabilities privately to security@tessaly.dev. Read [SECURITY.md](SECURITY.md) before you report.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) for the dev setup, testing rules and the security checklist. Developer docs live in [docs/](docs/), and the wiki pages in [docs/wiki/](docs/wiki/). Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md).

## License

Tessaly is licensed under the [Apache License 2.0](LICENSE). Maintained by 86-88 Solutions. Website: [tessaly.dev](https://tessaly.dev). Questions: hello@tessaly.dev.
