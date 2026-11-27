# Docker

Each release pushes a `tess` image for linux/amd64 and linux/arm64 to `ghcr.io/tessaly-dev/tessaly`. Tags are the version, such as `v0.1.0`, and `latest` for the newest full release.

## Run it

```sh
docker run -d --name tess \
  -p 127.0.0.1:7777:7777 \
  -v tess-data:/data \
  -v "$HOME/code:/home/tess/code" \
  ghcr.io/tessaly-dev/tessaly:latest
docker exec tess cat /data/token
```

Open <http://localhost:7777> and sign in with the token, then add a passkey on the Passkeys tab. Passkeys work only on `localhost`.

- Publish the port on `127.0.0.1` only, as above. Inside the container tess listens on `0.0.0.0` so Docker can reach it, and the `-p 127.0.0.1:` part keeps it off your network. For remote access, use an SSH tunnel, a VPN or a reverse proxy, as with the binary.
- Keep the host port the same as the container port. tess only accepts requests for `127.0.0.1:<port>` and `localhost:<port>`, so `-p 127.0.0.1:8080:7777` gets `403 forbidden host`. To use another port, change both: `-p 127.0.0.1:8080:8080` and add `serve --listen 0.0.0.0 --port 8080` at the end of `docker run`.
- `/data` holds the token, database, worktrees and logs. Keep it on a volume.
- Mount the repos you want tess to work on, then add them in the UI by their path inside the container, such as `/home/tess/code/my-app`.
- The container runs as user `tess` (uid 1000). Mounted repos must be writable by that uid.

## Add your agents

The image has git, gh and ssh, but no agent CLI. Build your own image on top of it:

```dockerfile
FROM ghcr.io/tessaly-dev/tessaly:v0.1.0
USER root
RUN apt-get update && apt-get install -y --no-install-recommends nodejs npm \
  && npm install -g <your-agent-cli> && rm -rf /var/lib/apt/lists/*
USER tess
```

Then add the agent to `/data/config.toml`, as in [Agents](Agents). Sign agents and gh in inside the container, for example with `docker exec -it tess gh auth login`, so their credentials stay in the container's home.

## Verify the image

```sh
cosign verify ghcr.io/tessaly-dev/tessaly:v0.1.0 \
  --certificate-identity-regexp '^https://github.com/Tessaly-dev/tessaly/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify oci://ghcr.io/tessaly-dev/tessaly:v0.1.0 --repo Tessaly-dev/tessaly
```

## Build it locally

`make snapshot` builds the image for both platforms with the binaries, without pushing. You need Docker with buildx.
