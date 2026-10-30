#!/usr/bin/env bash
# Regulus AI — build the release binaries with version stamping.
#
# Usage: bash scripts/build.bash [version]
#   version defaults to the package.json version.
#
# Cross-compiles the regulus core for the six supported platforms into
# binaries/ (distributed by the npm package; the launcher resolves the
# right one). The version is stamped via ldflags.

set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

version="${1:-$(node -p "require('./package.json').version")}"
stamp="main.version=${version}"

targets=(
  "darwin/arm64"
  "darwin/amd64"
  "linux/arm64"
  "linux/amd64"
  "windows/arm64"
  "windows/amd64"
)

mkdir -p binaries
for target in "${targets[@]}"; do
  os="${target%%/*}"
  arch="${target##*/}"
  ext=""
  if [ "$os" = "windows" ]; then
    ext=".exe"
  fi
  name="regulus-${os}-${arch}${ext}"
  echo "building binaries/${name} (${version})"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X ${stamp}" \
    -o "binaries/${name}" ./cmd/regulus
done

echo "done: $(ls binaries | wc -l) binaries in binaries/ (version ${version})"

# H3 (S4): the documented flow also refreshes the local binary, stamped
# with the same version. A raw `go build` stays unstamped by design
# (doctor warns); `npm run build` never is.
echo "building bin/regulus (${version})"
go build -trimpath \
  -ldflags "-s -w -X ${stamp}" \
  -o "bin/regulus" ./cmd/regulus

echo "local: bin/regulus (version ${version})"
