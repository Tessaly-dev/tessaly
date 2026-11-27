# Releasing

Pushing a version tag starts the release workflow. It builds `tess` for Linux and macOS on amd64 and arm64, signs the files and uploads a draft GitHub release. A maintainer checks the draft and publishes it.

## What a release holds

| File | What it is |
| --- | --- |
| `tess_<version>_<os>_<arch>.tar.gz` | The `tess` binary with the UI built in, plus `LICENSE` and `README.md`. One per `linux` or `darwin` and `amd64` or `arm64`. |
| `checksums.txt` | The SHA-256 of every archive |
| `checksums.txt.sigstore.json` | A keyless cosign signature of `checksums.txt`, made with the workflow's GitHub identity |
| `*.sbom.json` | An SPDX SBOM of each archive, made by syft |

The workflow also stores a build provenance attestation for every archive on GitHub.

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

The `release` environment in the repo settings gates the workflow. Add required reviewers there so each release waits for a second maintainer.

## Try a release build locally

You need [GoReleaser](https://goreleaser.com/install/) v2.

```sh
make snapshot
```

This writes the archives and `checksums.txt` to `dist/`, without signing or uploading anything. CI runs the same snapshot on every pull request, so a broken release config fails before a tag.

## Verify a download

Check the signature on the checksums, then the archive against the checksums. You need [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v2.4 or newer.

```sh
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/Tessaly-dev/tessaly/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --ignore-missing -c checksums.txt   # macOS: shasum -a 256 --ignore-missing -c checksums.txt
```

Or check the build provenance with the GitHub CLI:

```sh
gh attestation verify tess_<version>_linux_amd64.tar.gz --repo Tessaly-dev/tessaly
```

## Install script

`scripts/install.sh` installs the latest release, or the one in `TESS_VERSION`, into `~/.local/bin` (or `TESS_INSTALL_DIR`). It never uses sudo.

```sh
curl -fsSL https://raw.githubusercontent.com/Tessaly-dev/tessaly/main/scripts/install.sh | sh
```

It picks the archive for your OS and CPU, checks the cosign signature on `checksums.txt` when cosign is installed, and always checks the archive's SHA-256 before it installs. Set `TESS_REQUIRE_SIGNATURE=1` to stop when cosign is missing. `make test-install` runs its tests against a fake release, with no network.

## How the workflow stays safe

- It runs only on `v*` tags, and only for commits already on `main`.
- The workflow has no permissions by default. The release job gets `contents: write`, `id-token: write` and `attestations: write`, and nothing else.
- Every action is pinned to a full commit SHA, and GoReleaser to an exact version.
- It uses no dependency caches, so a cache written by another workflow cannot reach a release.
- Signing is keyless through GitHub's OIDC token. There is no signing key to leak.
