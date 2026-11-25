#!/usr/bin/env bash
# Verify Regulus inside WSL when Windows application-control policy
# (Smart App Control / WDAC) blocks freshly built executables.
#
# The Windows host can compile but never execute a new PE, so `go test`
# dies at fork/exec of the freshly linked test binary. Compilation is
# fine, execution is not. This script moves execution to the WSL
# distro, where no Windows policy applies.
#
# Nothing here needs Administrator: the toolchain lives in the distro's
# own $HOME ($HOME/.local/go). Reverting is `rm -rf $HOME/.local/go`.
#
# Usage:
#   scripts/verify-wsl.sh                    # go test -short ./...
#   scripts/verify-wsl.sh ./internal/pxexec/ # any go test arguments
#   scripts/verify-wsl.sh --gates            # install gate tools too
#   REGULUS_WSL_DISTRO=Debian scripts/verify-wsl.sh
#   REGULUS_WSL_GO=1.27.3 scripts/verify-wsl.sh
#
# Exit status is the test command's own status, so this composes with
# any CI or local gate that already calls `go test`.
set -euo pipefail

DISTRO="${REGULUS_WSL_DISTRO:-Ubuntu}"
GO_VERSION="${REGULUS_WSL_GO:-1.27.3}"
GREMLINS_VERSION="${REGULUS_GREMLINS_VERSION:-v0.6.0}"
NODE_VERSION="${REGULUS_NODE_VERSION:-22.11.0}"

with_gates=0
with_node=0
args=()
for arg in "$@"; do
	case "$arg" in
	--gates) with_gates=1 ;;
	--node) with_node=1 ;;
	*) args+=("$arg") ;;
	esac
done
if [ ${#args[@]} -eq 0 ]; then
	args=(-short ./...)
fi

# Repo root as a Windows path: WSL reaches it through /mnt/<drive>/...
if ! repo_root="$(pwd -W 2>/dev/null)"; then
	echo "verify-wsl: run this from Git Bash or WSL (pwd -W unavailable)" >&2
	exit 2
fi
repo_root="${repo_root%/}"

if ! command -v wsl.exe >/dev/null 2>&1; then
	echo "verify-wsl: wsl.exe not found; this host has no sandbox backend" >&2
	exit 2
fi

if ! wsl.exe -l -q 2>/dev/null | tr -d '\0\r' | grep -qx "$DISTRO"; then
	echo "verify-wsl: distro '$DISTRO' is not installed (wsl -l -q)" >&2
	exit 2
fi

gate_step=''
if [ "$with_gates" -eq 1 ]; then
	gate_step="command -v gremlins >/dev/null 2>&1 || go install github.com/go-gremlins/gremlins/cmd/gremlins@${GREMLINS_VERSION}"
fi

# --node: the agent-eval corpus ships JS fixtures and shells out to
# node. Same user-level bootstrap as Go, no Administrator either.
node_step=''
if [ "$with_node" -eq 1 ]; then
	node_step="if ! command -v node >/dev/null 2>&1; then
	echo 'verify-wsl: installing Node ${NODE_VERSION} linux-x64 into \$HOME/.local/node (no Administrator needed)' >&2
	mkdir -p \"\$HOME/.local\"
	curl -fsSL -o /tmp/regulus-node.tgz 'https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.gz'
	rm -rf \"\$HOME/.local/node\"
	tar -C \"\$HOME/.local\" -xzf /tmp/regulus-node.tgz
	ln -sfn \"\$HOME/.local/node-v${NODE_VERSION}-linux-x64\" \"\$HOME/.local/node\"
	rm -f /tmp/regulus-node.tgz
fi"
fi

# Everything below runs inside the distro: bootstrap, then the caller's
# go test arguments verbatim.
quoted_args=''
for a in "${args[@]}"; do
	quoted_args="${quoted_args} $(printf '%q' "$a")"
done

exec wsl.exe -d "$DISTRO" -e bash -lc "
set -euo pipefail
export PATH=\"\$HOME/.local/go/bin:\$HOME/go/bin:\$HOME/.local/node/bin:\$PATH\"

if ! command -v go >/dev/null 2>&1; then
	echo 'verify-wsl: installing Go ${GO_VERSION} linux-amd64 into \$HOME/.local/go (no Administrator needed)' >&2
	mkdir -p \"\$HOME/.local\"
	curl -fsSL -o /tmp/regulus-go.tgz 'https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz'
	rm -rf \"\$HOME/.local/go\"
	tar -C \"\$HOME/.local\" -xzf /tmp/regulus-go.tgz
	rm -f /tmp/regulus-go.tgz
fi

${gate_step}
${node_step}

cd \"\$(wslpath -a '$repo_root')\"
echo \"verify-wsl: \$(go version) in \$PWD\" >&2
go test${quoted_args}
"
