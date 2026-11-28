# Releasing

How maintainers cut a release. For how users install and verify Tessaly, see [Install](https://tessaly.dev/docs/install/) and [Verify downloads](https://tessaly.dev/docs/verify-downloads/) in the user docs.

Pushing a version tag starts the release workflow. It builds `tess` for Linux and macOS on amd64 and arm64, signs the files and uploads a draft GitHub release. A maintainer checks the draft and publishes it.

## What a release holds

| File | What it is |
| --- | --- |
| `tess_<version>_<os>_<arch>.tar.gz` | The `tess` binary with the UI built in, plus `LICENSE` and `README.md`. One per `linux` or `darwin` and `amd64` or `arm64`. |
| `checksums.txt` | The SHA-256 of every archive |
| `checksums.txt.sigstore.json` | A keyless cosign signature of `checksums.txt`, made with the workflow's GitHub identity |
| `*.sbom.json` | An SPDX SBOM of each archive, made by syft |

The workflow also stores a build provenance attestation for every archive on GitHub.

It pushes a multi-arch image to `ghcr.io/tessaly-dev/tessaly`, tagged with the version and, for full releases, `latest`. The image is signed with cosign by digest, has a buildkit SBOM and a provenance attestation pushed next to it. The image comes from the `Dockerfile`: it runs as user `tess` (uid 1000), sets `TESSALY_HOME=/data` and starts `tess serve --listen 0.0.0.0 --port 7777`. Users run it as in [Docker](https://tessaly.dev/docs/docker/).

There is no native Windows build. Windows users run the Linux build in WSL2.

## Cut a release

1. Make sure `main` is green and holds everything for the release.
2. Tag the commit on `main` and push the tag:

   ```sh
   git switch main && git pull
   git tag -a v0.1.0 -m "v0.1.0"
   git push origin v0.1.0
   ```

   A tag with a suffix, such as `v0.1.0-rc.1`, makes a pre-release.
3. Wait for the Release workflow to finish. It stops if the tagged commit is not on `main`.
4. Open the draft on the releases page, read the notes, then publish it.

The image is public as soon as the workflow pushes it, because a draft release cannot hold it back. After the very first release, set the `tessaly` package to public in the org's package settings, since GHCR creates new packages as private.

The `release` environment in the repo settings gates the workflow. Add required reviewers there so each release waits for a second maintainer.

## Try a release build locally

You need [GoReleaser](https://goreleaser.com/install/) v2.

```sh
make snapshot
```

This writes the archives and `checksums.txt` to `dist/` and builds the image, without signing, pushing or uploading anything. It needs Docker with buildx. CI runs the same snapshot on every pull request, so a broken release config fails before a tag.

## Verify a release

After you publish, check the signatures and attestations the way users do, as in [Verify downloads](https://tessaly.dev/docs/verify-downloads/).

## Homebrew

Each full release updates the `tess` cask in [Tessaly-dev/homebrew-tap](https://github.com/Tessaly-dev/homebrew-tap), which users install with `brew install tessaly-dev/tap/tess`. The workflow writes to the tap with `HOMEBREW_TAP_TOKEN`, a fine-grained token that can only write the tap's contents, stored in the `release` environment. Without that secret, and for pre-releases, the tap is left alone. The tap points at the release files, so publish the draft soon after the workflow ends, or `brew install` gets a 404 until you do.

The macOS binaries are not notarized by Apple, so the cask clears the quarantine flag after Homebrew has checked the archive's SHA-256.

## Install script

`scripts/install.sh` installs the latest release, or the one in `TESS_VERSION`, into `~/.local/bin` (or `TESS_INSTALL_DIR`). It never uses sudo. Users run it from `main` with `curl ... | sh`, so a change to it reaches users as soon as it merges. It picks the archive for your OS and CPU, checks the cosign signature on `checksums.txt` when cosign is installed, and always checks the archive's SHA-256 before it installs. Set `TESS_REQUIRE_SIGNATURE=1` to stop when cosign is missing. `make test-install` runs its tests against a fake release, with no network.

## How the workflow stays safe

- It runs only on `v*` tags, and only for commits already on `main`.
- The workflow has no permissions by default. The release job gets `contents: write`, `packages: write`, `id-token: write` and `attestations: write`, and nothing else.
- Every action is pinned to a full commit SHA, and GoReleaser to an exact version.
- It uses no dependency caches, so a cache written by another workflow cannot reach a release.
- Signing is keyless through GitHub's OIDC token. There is no signing key to leak.
