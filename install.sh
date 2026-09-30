#!/bin/sh
# Installs the latest ccshift release on Linux and macOS.
#
#   curl -fsSL https://www.timileyin.dev/ccshift/install.sh | sh
#   curl -fsSL https://www.timileyin.dev/ccshift/install.sh | sh -s -- --uninstall
#
# CCSHIFT_INSTALL_DIR  where the binary goes (default: ~/.local/bin)
# CCSHIFT_VERSION      a release tag such as v0.2.0 (default: the latest release)
# CCSHIFT_NO_SETUP=1   skip the question about running ccshift init
# NO_COLOR=1           plain output
set -eu

# Everything runs inside main, called on the last line, so a download that stops
# halfway cannot run part of the script.
main() {
	repo="timileyinpelumi/ccshift"
	dir="${CCSHIFT_INSTALL_DIR:-$HOME/.local/bin}"
	version="${CCSHIFT_VERSION:-latest}"
	colors

	if [ "${1:-}" = "--uninstall" ]; then
		uninstall
		return
	fi

	banner
	step "Checking this machine"
	case "$(uname -s)" in
		Linux) os=linux ;;
		Darwin) os=darwin ;;
		*) fail "there is no build for $(uname -s). On Windows, run install.ps1 in PowerShell." ;;
	esac
	case "$(uname -m)" in
		x86_64 | amd64) arch=amd64 ;;
		aarch64 | arm64) arch=arm64 ;;
		*) fail "there is no build for $(uname -m). Build from source: https://github.com/$repo" ;;
	esac
	for tool in curl tar; do
		command -v "$tool" >/dev/null 2>&1 || fail "$tool is needed and was not found."
	done
	done_ "$os $arch"

	if [ "$version" = latest ]; then
		base="https://github.com/$repo/releases/latest/download"
	else
		base="https://github.com/$repo/releases/download/$version"
	fi
	asset="ccshift_${os}_$arch.tar.gz"
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT

	step "Downloading $asset"
	spin curl -fsSL "$base/$asset" -o "$tmp/$asset" || fail "could not download $base/$asset"
	spin curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || fail "could not download $base/checksums.txt"
	done_ "$(size "$tmp/$asset")"

	step "Checking the checksum"
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
	done_ "sha256 matches"

	step "Installing to $dir"
	tar -xzf "$tmp/$asset" -C "$tmp" ccshift
	mkdir -p "$dir"
	install -m 0755 "$tmp/ccshift" "$dir/ccshift"
	done_ "ccshift $("$dir/ccshift" version)"

	case ":$PATH:" in
		*":$dir:"*) on_path=yes ;;
		*) on_path=no ;;
	esac

	echo
	printf '%s%sccshift is installed.%s\n' "$bold" "$green" "$reset"
	if [ "$on_path" = no ]; then
		echo
		printf '%sAdd it to your PATH%s by putting this line in your shell profile:\n' "$bold" "$reset"
		printf '  %sexport PATH="%s:$PATH"%s\n' "$cyan" "$dir" "$reset"
	fi

	# The script's stdin is the download, so the answer is read from the terminal.
	if [ -z "${CCSHIFT_NO_SETUP:-}" ] && [ -r /dev/tty ] && [ -t 1 ]; then
		echo
		printf 'Turn on autosave now? It adds three hooks to Claude Code and backs up your settings. [Y/n] '
		answer=$(head -n 1 </dev/tty 2>/dev/null || true)
		case "$answer" in
			n* | N*) ;;
			*)
				echo
				"$dir/ccshift" init && setup_done=yes || true
				;;
		esac
	fi

	echo
	printf '%sNext%s\n' "$bold" "$reset"
	if [ "${setup_done:-}" != yes ]; then
		printf '  %sccshift init%s       turn on autosave\n' "$cyan" "$reset"
	fi
	printf '  %sccshift doctor%s     check the setup\n' "$cyan" "$reset"
	printf '  %sccshift restore%s    bring your sessions back after a restart\n' "$cyan" "$reset"
	echo
	printf '%sDocs: https://www.timileyin.dev/ccshift%s\n' "$dim" "$reset"
}

uninstall() {
	banner
	bin="$dir/ccshift"
	[ -x "$bin" ] || bin=$(command -v ccshift 2>/dev/null || true)
	[ -n "$bin" ] || fail "ccshift was not found in $dir or on your PATH."
	# ccshift uninstall does the work: hooks, saved data and the binary itself.
	"$bin" uninstall </dev/tty
}

colors() {
	if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
		reset=$(printf '\033[0m')
		bold=$(printf '\033[1m')
		dim=$(printf '\033[2m')
		green=$(printf '\033[32m')
		red=$(printf '\033[31m')
		cyan=$(printf '\033[36m')
		amber=$(printf '\033[33m')
	else
		reset="" bold="" dim="" green="" red="" cyan="" amber=""
	fi
}

banner() {
	echo
	printf '%s' "$amber"
	cat <<'ART'
                  __    _ ______
  _______________/ /_  (_) __/ /_
 / ___/ ___/ ___/ __ \/ / /_/ __/
/ /__/ /__(__  ) / / / / __/ /_
\___/\___/____/_/ /_/_/_/  \__/
ART
	printf '%s\n' "$reset"
	printf '%sKeep your Claude Code sessions across restarts.%s\n\n' "$dim" "$reset"
}

step() { printf '%s›%s %s' "$cyan" "$reset" "$1"; }
done_() { printf '  %s✓ %s%s\n' "$green" "$1" "$reset"; }

fail() {
	printf '\n%s✗ %s%s\n' "$red" "$*" "$reset" >&2
	exit 1
}

# spin shows a spinner while a command runs, when there is a terminal to show it on.
spin() {
	if [ ! -t 1 ]; then
		"$@"
		return
	fi
	"$@" &
	pid=$!
	i=0
	while kill -0 "$pid" 2>/dev/null; do
		case $((i % 4)) in 0) c='|' ;; 1) c='/' ;; 2) c='-' ;; 3) c='\' ;; esac
		printf ' %s%s%s\b\b' "$dim" "$c" "$reset"
		i=$((i + 1))
		sleep 0.1
	done
	printf '  \b\b'
	wait "$pid"
}

size() {
	bytes=$(wc -c <"$1" | tr -d ' ')
	echo "$((bytes / 1024 / 1024)).$(((bytes / 1024 % 1024) * 10 / 1024)) MB"
}

main "$@"
