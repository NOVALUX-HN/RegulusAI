#!/usr/bin/env bash
# NOV-79: run Go benchmarks and compare against bench/perf-baseline.json.
# Single run, per-benchmark max_regression ratio; fails closed on breach.
# benchstat adoption (multi-rep statistics) is the documented next step
# once 3+ scheduled runs establish variability bands.
#
# Usage: bash scripts/perf-check.sh [--update] [--report FILE]
#   --update  regenerate bench/perf-baseline.json from this run
#             (intentional changes only, committed as its own change).
#   --report FILE  also write a Markdown report for CI artifacts.
set -euo pipefail
cd "$(dirname "$0")/.."

UPDATE=0
REPORT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --update) UPDATE=1 ;;
    --report) REPORT="${2:?--report needs FILE}"; shift ;;
    *) echo "usage: bash scripts/perf-check.sh [--update] [--report FILE]" >&2; exit 2 ;;
  esac
  shift
done

OUT=$(mktemp -d)
trap 'rm -rf "$OUT"' EXIT

run_bench() { # pkg benchtime outfile
  go test "./$1" -run XXX -bench . -benchtime "$2" -count 1 >"$OUT/$3" 2>&1
}
run_bench internal/almanac 100x almanac.txt
run_bench internal/trace 100x trace.txt
run_bench internal/cli 30x cli.txt

MODE="$UPDATE" BASE="bench/perf-baseline.json" REPORT_FILE="$REPORT" OUT_DIR="$OUT" python3 - <<'EOF'
import json, re, sys, os

with open("bench/perf-baseline.json") as f:
    base = json.load(f)
bl = base["benchmarks"]
update = os.environ.get("MODE", "0") == "1"
outdir = os.environ["OUT_DIR"]
report_path = os.environ.get("REPORT_FILE", "")

measured = {}
for fn in ("almanac.txt", "trace.txt", "cli.txt"):
    with open(os.path.join(outdir, fn)) as f:
        for line in f:
            m = re.match(r"^(Benchmark\w+)-\d+\s+\d+\s+([\d.]+)\s+ns/op", line)
            if m:
                measured[m.group(1)] = float(m.group(2))

if update:
    for name, ns in measured.items():
        if name in bl:
            bl[name]["ns_per_op"] = ns
    with open("bench/perf-baseline.json", "w") as f:
        json.dump(base, f, indent=2)
        f.write("\n")
    print("baseline regenerated from this run (review the diff before committing)")
    sys.exit(0)

breach = []
lines = ["# Perf check (NOV-79)", ""]
for name in sorted(set(list(bl) + list(measured))):
    if name not in measured:
        lines.append(f"- {name}: NOT RUN (fixture missing?)")
        continue
    if name not in bl:
        lines.append(f"- {name}: no baseline (add it to bench/perf-baseline.json)")
        continue
    ratio = measured[name] / bl[name]["ns_per_op"]
    flag = "BREACH" if ratio > bl[name]["max_regression"] else "ok"
    lines.append(f"- {name}: {measured[name]:.0f} ns/op vs {bl[name]['ns_per_op']:.0f} (x{ratio:.2f}, max x{bl[name]['max_regression']}) [{flag}]")
    if flag == "BREACH":
        breach.append(name)

report = "\n".join(lines) + "\n"
print(report)
if report_path:
    with open(report_path, "w") as f:
        f.write(report)
if breach:
    print("PERF BREACH: " + ", ".join(breach), file=sys.stderr)
    sys.exit(1)
print("PERF OK")
EOF
