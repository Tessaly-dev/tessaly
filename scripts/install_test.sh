#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
#
# Tests for install.sh. They serve a fake release from a temp folder over
# file:// and use a fake cosign, so they need no network. Run: make test-install
set -eu

script=$(cd "$(dirname "$0")" && pwd)/install.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; esac
case "$(uname -m)" in x86_64 | amd64) arch=amd64 ;; *) arch=arm64 ;; esac
archive="tess_9.9.9_${os}_${arch}.tar.gz"

# A fake release: a tess that prints its version, checksums and a bundle.
mkdir -p "$work/build" "$work/release" "$work/bin"
printf '#!/bin/sh\necho tess v9.9.9\n' >"$work/build/tess"
chmod +x "$work/build/tess"
tar -czf "$work/release/$archive" -C "$work/build" tess
(cd "$work/release" && { sha256sum "$archive" 2>/dev/null || shasum -a 256 "$archive"; } >checksums.txt)
echo '{}' >"$work/release/checksums.txt.sigstore.json"

# A fake cosign that passes unless COSIGN_FAIL is set.
# shellcheck disable=SC2016 # the fake script expands it, not this one
printf '#!/bin/sh\n[ -z "${COSIGN_FAIL:-}" ]\n' >"$work/bin/cosign"
chmod +x "$work/bin/cosign"

failed=0
run() {
	PATH="$work/bin:$PATH" TESS_VERSION=v9.9.9 TESS_DOWNLOAD_URL="file://$work/release" \
		TESS_INSTALL_DIR="$work/install" "$@" sh "$script" >"$work/out" 2>&1
}
pass() { printf 'ok   %s\n' "$1"; }
bad() { printf 'FAIL %s\n' "$1"; cat "$work/out"; failed=1; }

expect_ok() {
	name=$1; shift
	rm -rf "$work/install"
	if run "$@" && [ "$("$work/install/tess")" = "tess v9.9.9" ]; then pass "$name"; else bad "$name"; fi
}

expect_fail() {
	name=$1 want=$2; shift 2
	rm -rf "$work/install"
	if run "$@"; then bad "$name: install should fail"; return; fi
	if [ -e "$work/install/tess" ]; then bad "$name: tess was installed"; return; fi
	if grep -q "$want" "$work/out"; then pass "$name"; else bad "$name: output lacks '$want'"; fi
}

expect_ok "installs a release" env
expect_fail "rejects a bad signature" "signature on checksums.txt is not valid" env COSIGN_FAIL=1
expect_fail "rejects a malformed version" "must look like v0.1.0" env TESS_VERSION='v1;rm -rf /'

cp "$work/release/checksums.txt" "$work/good"
sed 's/^[0-9a-f]*/0000000000000000000000000000000000000000000000000000000000000000/' "$work/good" >"$work/release/checksums.txt"
expect_fail "rejects a wrong SHA-256" "does not match" env
: >"$work/release/checksums.txt"
expect_fail "rejects an archive missing from checksums" "is not in checksums.txt" env
cp "$work/good" "$work/release/checksums.txt"

exit "$failed"
