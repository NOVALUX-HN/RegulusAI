#!/usr/bin/env bash
# Regulus AI — install regulus on PATH (bash mirror of scripts/install-devin.ps1).
#
# Usage: bash scripts/install-devin.bash
#
# What it does (same S1+S2 as the .ps1, minimal portable form):
#   1. Builds bin/regulus from ./cmd/regulus, stamping main.version from
#      package.json:
#        go build -trimpath -ldflags "-X main.version=<package.json>" -o bin/regulus ./cmd/regulus
#   2. Installs regulus into ~/.local/bin, which must already be on PATH
#      (no writable-PATH-dir fallback: add ~/.local/bin to PATH when the
#      script tells you to). Windows PowerShell users run
#      scripts/install-devin.ps1 instead (same checks, ~/bin semantics).
#   3. Verifies S1: `regulus contract` prints regulus.contract/v1 and
#      `regulus --version` reports the stamped version.
#   4. Verifies S2: `regulus mcp` answers an initialize request over stdio
#      with serverInfo.contract = regulus.contract/v1 (30s timeout).
#
# Pure bash + coreutils (timeout, grep -F, install). Exit 0 on success, 1
# with a message on stderr otherwise. Requires Go and node on PATH.

set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

fail() {
  echo "install-devin: $1" >&2
  exit 1
}

# 0. Go, node (package.json version) and timeout (30s MCP smoke) are
# required; fail loud instead of producing a stale binary.
command -v go >/dev/null 2>&1 || fail "Go toolchain not found on PATH. Install Go (https://go.dev/dl/) and re-run this script."
command -v node >/dev/null 2>&1 || fail "node not found on PATH (needed to read the version from package.json)."
command -v timeout >/dev/null 2>&1 || fail "timeout not found on PATH (GNU coreutils, needed for the 30s MCP smoke)."

# 1. Build with the package.json version stamped in.
version="$(node -p "require('./package.json').version")"
[ -n "$version" ] || fail "could not read a version from package.json."
echo "building bin/regulus (version ${version})"
go build -trimpath -ldflags "-X main.version=${version}" -o "bin/regulus" ./cmd/regulus \
  || fail "go build failed."

# 2. Install into ~/.local/bin, which must already be on PATH.
destdir="$HOME/.local/bin"
case ":$PATH:" in
  *":${destdir}:"*) ;;
  *) fail "no writable directory on PATH: ${destdir} is not on PATH — add 'export PATH=\"\$HOME/.local/bin:\$PATH\"' to ~/.bashrc, restart the shell, and re-run." ;;
esac
mkdir -p "$destdir"
install -m755 "bin/regulus" "${destdir}/regulus"
echo "installed: ${destdir}/regulus"

# 3. S1: contract + stamped version, resolved via PATH like a consumer.
[ "$(command -v regulus)" = "${destdir}/regulus" ] \
  || echo "warning: PATH resolves regulus to $(command -v regulus), not ${destdir}/regulus (shadowing install)"
contract_out="$(regulus contract 2>&1)" || fail "regulus contract failed: ${contract_out}"
[ "$contract_out" = "regulus.contract/v1" ] || fail "S1 contract check failed: regulus contract printed '$contract_out', want 'regulus.contract/v1'."
echo "contract: ${contract_out}"
version_out="$(regulus --version 2>&1)" || fail "regulus --version failed: ${version_out}"
[ "$version_out" = "regulus ${version}" ] || fail "S1 version check failed: regulus --version printed '$version_out', want 'regulus ${version}'."
echo "version: ${version_out}"

# 4. S2: stdio initialize smoke — serverInfo.contract must be v1 (30s cap).
# pipefail is off for this pipeline on purpose: head exits after the first
# line and the MCP server dies of SIGPIPE, which must not fail the check;
# an empty first line (timeout) fails it below instead.
init='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"install-devin-smoke","version":"0"}}}'
set +o pipefail
line="$(printf '%s\n' "$init" | timeout 30 regulus mcp 2>/dev/null | head -n 1)"
set -o pipefail
[ -n "$line" ] || fail "S2 MCP smoke failed: no initialize response within 30s."
printf '%s\n' "$line" | grep -qF 'regulus.contract/v1' \
  || fail "S2 MCP smoke failed: serverInfo.contract is not regulus.contract/v1. Raw: $line"
echo "mcp initialize: serverInfo.contract = regulus.contract/v1"

echo "done: regulus ${version} on PATH (${destdir}/regulus), contract + MCP initialize verified."
