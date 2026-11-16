#!/usr/bin/env bash
# NOV-84 Paso 64: per-package coverage ratchet over the gate-produced
# coverprofile. Fails closed below the floors in bench/coverage-floors.json.
# Missing coverprofile (unit-tests gate never produced it) warns and passes:
# that gate already failed on its own.
set -euo pipefail
cd "$(dirname "$0")/.."

COV="${1:-.regulus/coverage.out}"
FLOORS="bench/coverage-floors.json"

if [ ! -f "$COV" ]; then
  echo "coverage floors: no $COV (unit-tests gate did not produce it); skipping (its own failure already recorded)"
  exit 0
fi

python3 - "$COV" "$FLOORS" <<'EOF'
import json, sys
from collections import defaultdict

cov_path, floors_path = sys.argv[1], sys.argv[2]
floors = json.load(open(floors_path))["floors"]
stmts = defaultdict(int)
covered = defaultdict(int)
with open(cov_path) as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith("mode:"):
            continue
        # path/to/file.go:Func N stmts XX.X%  (tokens: loc N "stmts" pct)
        parts = line.split()
        if len(parts) < 4:
            continue
        pkg = "/".join(parts[0].split(":")[0].split("/")[:2])
        try:
            n = int(parts[-3])
            pct = float(parts[-1].rstrip("%"))
        except ValueError:
            continue
        stmts[pkg] += n
        covered[pkg] += n * pct / 100.0

breach = []
print("per-package coverage (NOV-84 ratchet):")
for pkg in sorted(set(list(floors) + [p for p in stmts if stmts[p] > 0])):
    if stmts[pkg] == 0:
        print(f"- {pkg}: no statements measured (skipped)")
        continue
    got = 100.0 * covered[pkg] / stmts[pkg]
    want = floors.get(pkg)
    if want is None:
        print(f"- {pkg}: {got:.1f}% (no floor)")
        continue
    flag = "BREACH" if got < want else "ok"
    print(f"- {pkg}: {got:.1f}% vs floor {want}% [{flag}]")
    if flag == "BREACH":
        breach.append(pkg)

if breach:
    print("COVERAGE FLOORS BREACH: " + ", ".join(breach), file=sys.stderr)
    sys.exit(1)
print("COVERAGE FLOORS OK")
EOF
