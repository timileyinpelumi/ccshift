#!/bin/sh
# Installs the latest ccshift release for this machine.
#
#   curl -fsSL https://www.timileyin.dev/ccshift/install.sh | sh
#
# CCSHIFT_INSTALL_DIR  where the binary goes (default: ~/.local/bin)
# CCSHIFT_VERSION      a release tag such as v0.1.0 (default: the latest release)
set -eu

# Everything runs inside main, called on the last line, so a download that stops
# halfway cannot run part of the script.
main() {
	repo="timileyinpelumi/ccshift"
	dir="${CCSHIFT_INSTALL_DIR:-$HOME/.local/bin}"
	version="${CCSHIFT_VERSION:-latest}"

	case "$(uname -s)" in
		Linux) os=linux ;;
		Darwin) os=darwin ;;
		*) fail "no build for $(uname -s). On Windows use install.ps1: https://github.com/$repo#install" ;;
	esac
	case "$(uname -m)" in
		x86_64 | amd64) arch=amd64 ;;
		aarch64 | arm64) arch=arm64 ;;
		*) fail "there is no build for $(uname -m). Build from source: https://github.com/$repo" ;;
	esac
	for tool in curl tar; do
		command -v "$tool" >/dev/null 2>&1 || fail "$tool is needed and was not found."
	done

	if [ "$version" = latest ]; then
		base="https://github.com/$repo/releases/latest/download"
	else
		base="https://github.com/$repo/releases/download/$version"
	fi
	asset="ccshift_${os}_$arch.tar.gz"
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT

	echo "Downloading $asset"
	curl -fsSL "$base/$asset" -o "$tmp/$asset" || fail "could not download $base/$asset"
	curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || fail "could not download $base/checksums.txt"

	want=$(awk -v f="$asset" '$2 == f { print $1 }' "$tmp/checksums.txt")
	[ -n "$want" ] || fail "checksums.txt has no entry for $asset"
	if command -v sha256sum >/dev/null 2>&1; then
		got=$(sha256sum "$tmp/$asset" | awk '{ print $1 }')
	elif command -v shasum >/dev/null 2>&1; then
		got=$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')
	else
		fail "sha256sum is needed to check the download and was not found."
	fi
	[ "$want" = "$got" ] || fail "the download does not match its checksum. Nothing was installed."

	tar -xzf "$tmp/$asset" -C "$tmp" ccshift
	mkdir -p "$dir"
	install -m 0755 "$tmp/ccshift" "$dir/ccshift"
	echo "Installed ccshift $("$dir/ccshift" version) to $dir/ccshift"

	case ":$PATH:" in
		*":$dir:"*) ;;
		*) echo "$dir is not on your PATH. Add this line to your shell profile:"
		   echo "  export PATH=\"$dir:\$PATH\"" ;;
	esac
	echo "Next: ccshift init"
}

fail() {
	echo "ccshift install: $*" >&2
	exit 1
}

main "$@"
