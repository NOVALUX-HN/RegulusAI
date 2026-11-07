#!/usr/bin/env bash
# Regulus AI — install regulus on PATH (bash mirror of scripts/install-devin.ps1).
#
# Usage: bash scripts/install-devin.bash
#
# What it does (S1+S2 as the .ps1, plus S3 Devin surface; minimal portable form):
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
# with a message on stderr otherwise. Requires Go and timeout on PATH;
# node is optional (used for the version when present, pure bash
# grep/sed fallback otherwise).

set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

fail() {
  echo "install-devin: $1" >&2
  exit 1
}

# 0. Go (build) and timeout (30s MCP smoke) are required; fail loud
# instead of producing a stale binary. node is optional: the version
# comes from node when present, else the pure bash fallback below.
command -v go >/dev/null 2>&1 || fail "Go toolchain not found on PATH. Install Go (https://go.dev/dl/) and re-run this script."
command -v timeout >/dev/null 2>&1 || fail "timeout not found on PATH (GNU coreutils, needed for the 30s MCP smoke)."

# G5: version without node — pure bash/grep/sed fallback.
# Happy path unchanged: node wins when present.
pkg_version_fallback() {
  grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' package.json 2>/dev/null \
    | head -n 1 \
    | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
}

# 1. Build with the package.json version stamped in.
if command -v node >/dev/null 2>&1; then
  version="$(node -p "require('./package.json').version")"
else
  version="$(pkg_version_fallback)"
fi
[ -n "${version:-}" ] || fail "could not read a version from package.json."
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

# 5. S3: Devin surface (headless-safe: `skills`/`mcp` need no auth).
# 5a. The Devin CLI must exist. It is NOT installed here on purpose: on
#     some machines it comes from a system mise config that other machines
#     lack, so fail loud with the exact remedy instead of guessing.
if ! command -v devin >/dev/null 2>&1; then
  cat >&2 <<'EOF'
install-devin: S3 needs the Devin CLI and it is not on PATH.
Remedy (pick one), then re-run this script:
  mise (recommended): add to ~/.config/mise/config.toml and run `mise install devin`:
    [tools]
    "http:devin" = { version = "latest", platforms_linux_x64_url = "https://static.devin.ai/cli/{{ version }}/devin-{{ version }}-x86_64-unknown-linux.tar.gz", platforms_linux_arm64_url = "https://static.devin.ai/cli/{{ version }}/devin-{{ version }}-aarch64-unknown-linux.tar.gz", bin_path = "bin", version_list_url = "https://static.devin.ai/cli/current/manifest.json", version_json_path = ".version" }
  manual: fetch the current version from https://static.devin.ai/cli/current/manifest.json (.version), unpack the matching tarball above into ~/.local/bin, and ensure it is on PATH.
EOF
  exit 1
fi
echo "devin: $(devin --version 2>&1 | head -n 1)"
# 5b. The /regulus selectable skill must resolve from this repo.
devin skills show regulus >/dev/null 2>&1 \
  || fail "S3 skill check failed: /regulus not found — run from the repo root with .devin/skills/regulus/SKILL.md present."
echo "skill: /regulus selectable (user-triggered only)"
# 5c. MCP registration, idempotent: add only when missing so a present
#     server never rewrites the committed .devin/mcp_config.json.
if devin mcp get regulus >/dev/null 2>&1; then
  echo "mcp: regulus already registered"
else
  devin mcp add -s project regulus -- regulus mcp \
    || fail "S3 MCP registration failed: devin mcp add regulus."
  echo "mcp: regulus registered"
fi

echo "done: regulus ${version} on PATH (${destdir}/regulus), contract + MCP initialize verified."
