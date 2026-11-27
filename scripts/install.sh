#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
#
# Install tess from a GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/Tessaly-dev/tessaly/main/scripts/install.sh | sh
#
# It downloads the archive for this OS and CPU, checks its SHA-256 against
# checksums.txt, checks the cosign signature on checksums.txt when cosign is
# installed, and puts tess in ~/.local/bin. It never uses sudo.
#
# Settings, all optional:
#   TESS_VERSION      version to install, such as v0.1.0 (default: latest)
#   TESS_INSTALL_DIR  folder for the binary (default: ~/.local/bin)
#   TESS_REQUIRE_SIGNATURE=1  stop if cosign is missing
#   TESS_DOWNLOAD_URL base URL of the release files, for mirrors and tests
set -eu

REPO="Tessaly-dev/tessaly"
IDENTITY="^https://github.com/${REPO}/\.github/workflows/release\.yml@refs/tags/v"
ISSUER="https://token.actions.githubusercontent.com"

say() { printf 'tess install: %s\n' "$*" >&2; }
fail() { say "$*"; exit 1; }

need() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

detect_os() {
	case "$(uname -s)" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	*) fail "$(uname -s) is not supported. On Windows, run this in WSL2." ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*) fail "CPU $(uname -m) is not supported. tess builds for amd64 and arm64." ;;
	esac
}

# latest_version reads the tag that github.com/<repo>/releases/latest
# redirects to, so it needs no API token and no JSON parser.
latest_version() {
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest") ||
		fail "could not find the latest release"
	tag=${url##*/}
	valid_version "$tag" || fail "no published release found"
	echo "$tag"
}

# valid_version accepts v1.2.3 and v1.2.3-rc.1, and nothing that could
# change a URL or a path.
valid_version() {
	printf '%s' "$1" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$'
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		fail "sha256sum or shasum is required"
	fi
}

verify_signature() {
	if ! command -v cosign >/dev/null 2>&1; then
		[ "${TESS_REQUIRE_SIGNATURE:-0}" = 1 ] && fail "cosign is required to check the signature"
		say "cosign not found, so the signature was not checked. The SHA-256 was checked."
		return
	fi
	cosign verify-blob \
		--bundle "$1/checksums.txt.sigstore.json" \
		--certificate-identity-regexp "$IDENTITY" \
		--certificate-oidc-issuer "$ISSUER" \
		"$1/checksums.txt" >/dev/null 2>&1 || fail "the signature on checksums.txt is not valid"
	say "signature checked with cosign"
}

main() {
	need curl
	need tar
	need uname

	os=$(detect_os)
	arch=$(detect_arch)
	version=${TESS_VERSION:-}
	[ -n "$version" ] || version=$(latest_version)
	valid_version "$version" || fail "TESS_VERSION must look like v0.1.0, got '$version'"
	base=${TESS_DOWNLOAD_URL:-https://github.com/${REPO}/releases/download/${version}}
	dir=${TESS_INSTALL_DIR:-$HOME/.local/bin}
	archive="tess_${version#v}_${os}_${arch}.tar.gz"

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM

	say "downloading tess ${version} for ${os}/${arch}"
	for f in "$archive" checksums.txt checksums.txt.sigstore.json; do
		curl -fsSL --proto '=https,file' -o "$tmp/$f" "$base/$f" || fail "could not download $base/$f"
	done

	verify_signature "$tmp"

	want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
	[ -n "$want" ] || fail "$archive is not in checksums.txt"
	[ "$(sha256 "$tmp/$archive")" = "$want" ] || fail "SHA-256 of $archive does not match checksums.txt"

	tar -xzf "$tmp/$archive" -C "$tmp" tess
	mkdir -p "$dir"
	# Move into place in one step, so a running tess is never half written.
	cp "$tmp/tess" "$dir/.tess.new"
	chmod 0755 "$dir/.tess.new"
	mv -f "$dir/.tess.new" "$dir/tess"

	say "installed $("$dir/tess" version) to $dir/tess"
	case ":$PATH:" in
	*":$dir:"*) ;;
	*) say "add $dir to your PATH to run tess" ;;
	esac
}

main "$@"
