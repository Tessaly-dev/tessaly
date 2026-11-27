# Remote access

`tess serve` listens on `127.0.0.1` and accepts requests only for `127.0.0.1:<port>` and `localhost:<port>`. To use Tessaly from another device, pick one of these. Each one keeps sign-in on every request: there is no bypass for local or proxied requests.

## SSH tunnel

```sh
ssh -L 7777:127.0.0.1:7777 you@host
```

Then open `http://localhost:7777` on your device. Passkeys work, because the browser sees `localhost`. Nothing changes on the host.

## HTTPS reverse proxy

Put a reverse proxy with a TLS certificate in front of tess, and tell tess the address in `~/.tessaly/config.toml`:

```toml
public_url = "https://tess.example.com"
```

Put `public_url` above any `[agents.*]` table, since TOML reads keys after a table header as part of that table. Restart `tess serve`. It prints `Also open at https://tess.example.com`. Then:

- The Host check also accepts `tess.example.com`.
- Passkeys work at `https://tess.example.com`. A passkey belongs to the address it was made on, so add one there too if you also use `localhost`.
- The session cookie is `Secure` on that address.
- The terminal works, because its WebSocket comes from the same address.

The proxy must forward the original `Host` header and WebSocket upgrades. Caddy does both by default:

```
tess.example.com {
	reverse_proxy 127.0.0.1:7777
}
```

With nginx:

```nginx
location / {
    proxy_pass http://127.0.0.1:7777;
    proxy_set_header Host $http_host;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

Tessaly does not read `X-Forwarded-*` headers, so you do not need to list trusted proxies.

`public_url` takes only a scheme and a host, with an optional port, such as `https://tess.example.com:8443`. Tessaly serves the UI at the root of that host, not under a path.

## VPN name without a proxy

To reach tess by a VPN address directly, listen on that address and set `public_url` to the name you open:

```sh
tess serve --listen 100.64.0.5
```

```toml
public_url = "http://tess-host.vpn.example:7777"
```

Browsers allow passkeys only on HTTPS or `localhost`, so passkeys do not work at an `http://` address. Sign in there with the token, which works only while you have no passkey or run `tess serve --allow-token-login`. To use passkeys over a VPN, put an HTTPS reverse proxy in front instead, or use an SSH tunnel.
