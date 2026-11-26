#!/usr/bin/env bash
# Run any Regulus command inside WSL when the Windows host blocks
# freshly built executables.
#
# The root problem is not "tests do not run": on a host with Smart App
# Control (or WDAC) in enforcement, `regulus.exe` itself cannot start,
# because every newly linked PE is refused. Moving only the test
# binaries would leave the product unusable, so this runs the whole
# Linux build of Regulus inside the distro, where no Windows
# application-control policy exists.
#
# Source stays where it is: the distro reads the same working tree
# through /mnt/<drive>/, so there is one repository, not a copy.
#
# Nothing here needs Administrator. The toolchain and the built binary
# live in the distro's $HOME. Revert with:
#   rm -rf ~/.local/go ~/.local/node ~/go ~/.regulus
#
# Usage:
#   scripts/regulus-wsl.sh doctor
#   scripts/regulus-wsl.sh gate run <task>
#   scripts/regulus-wsl.sh memory context
#   REGULUS_WSL_DISTRO=Debian scripts/regulus-wsl.sh status
#
# Exit status is the command's own status.
set -euo pipefail

DISTRO="${REGULUS_WSL_DISTRO:-Ubuntu}"
GO_VERSION="${REGULUS_WSL_GO:-1.27.3}"

if [ $# -eq 0 ]; then
	echo "usage: $(basename "$0") <regulus command...>" >&2
	exit 2
fi

if ! command -v wsl.exe >/dev/null 2>&1; then
	echo "regulus-wsl: wsl.exe not found; run the native binary instead" >&2
	exit 2
fi
if ! wsl.exe -l -q 2>/dev/null | tr -d '\0\r' | grep -qx "$DISTRO"; then
	echo "regulus-wsl: distro '$DISTRO' is not installed (wsl -l -q)" >&2
	exit 2
fi

cd "$(dirname "$0")/.."
if ! repo_root="$(pwd -W 2>/dev/null)"; then
	echo "regulus-wsl: run this from Git Bash or WSL (pwd -W unavailable)" >&2
	exit 2
fi

# Values travel as positional arguments, never interpolated into the
# inner script: quoting a nested script by hand is how escaping bugs
# sneak in.
exec wsl.exe -d "$DISTRO" -e bash -lc '
set -euo pipefail
BIN_DIR="$HOME/.regulus/bin"
export PATH="$HOME/.local/go/bin:$HOME/go/bin:$HOME/.local/node/bin:$BIN_DIR:$PATH"
GO_VERSION="$1"; shift
repo_root="$1"; shift

if ! command -v go >/dev/null 2>&1; then
	echo "regulus-wsl: installing Go $GO_VERSION linux-amd64 into $HOME/.local/go (no Administrator needed)" >&2
	mkdir -p "$HOME/.local"
	curl -fsSL -o /tmp/regulus-go.tgz "https://go.dev/dl/go$GO_VERSION.linux-amd64.tar.gz"
	rm -rf "$HOME/.local/go"
	tar -C "$HOME/.local" -xzf /tmp/regulus-go.tgz
	rm -f /tmp/regulus-go.tgz
fi

cd "$(wslpath -a "$repo_root")"

# Build into the distro own filesystem: the 9p mount is fine for
# reading sources, but a binary written there is slow and pointless.
mkdir -p "$BIN_DIR"
if [ ! -x "$BIN_DIR/regulus" ] || [ package.json -nt "$BIN_DIR/regulus" ] || [ -n "$(find cmd internal -name "*.go" -newer "$BIN_DIR/regulus" -print -quit)" ]; then
	# Stamp the version the same way scripts/build.bash does, so doctor and
	# every receipt can attribute the binary instead of reporting an
	# unstamped dev build.
	version="$(sed -n "s/.*\"version\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" package.json | head -n 1)"
	[ -n "$version" ] || version="dev"
	echo "regulus-wsl: building linux/amd64 regulus $version" >&2
	go build -ldflags "-X main.version=$version" -o "$BIN_DIR/regulus" ./cmd/regulus
fi

exec "$BIN_DIR/regulus" "$@"
' regulus-wsl "$GO_VERSION" "$repo_root" "$@"
