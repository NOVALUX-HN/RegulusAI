# Quality tooling decisions — F20.2 (NOV-151)

Record of what was evaluated, what was adopted, and what was rejected **with the
reason**. F20.2 asks for complexity/coverage/CRAP, duplication and dead-code
measurement, and explicitly forbids adopting tooling that does not justify its cost.

Read this with `docs/quality-intelligence.md`: that document covers **degradation over
time** (baselines, windows, diagnosis from real runs). This one covers **static
structure**. They are different questions and must not be merged.

## The constraint that decides most of this

`go.mod` declares **no `require` at all**. Regulus is a stdlib-only module. Adding any
analysis tool therefore means adding the project's first external dependency — a
delivery decision (installers, CI, supply chain), not a quality one. Several candidates
below are rejected on exactly that ground, and that is stated rather than implied.

## Already measured and gated — keep, do not rebuild

| Signal | Where | Gate |
|---|---|---|
| Complexity + coverage → CRAP | `internal/crap` (`CRAP`, `Analyze`) | `crap` — **required**, `--max 30 --max-critical 30 --max-standard 50` |
| Coverage floor | `go tool cover -func` | `coverage-floor` — **required**, `>= 80%` |
| Unit size | `regulus size check` | `size` — **required** |
| Static checks | `go vet` with 30+ analysers | `lint` — **required** |

The F20.1 protocol table records the size gate as the enforced half of "small units".
Nothing here changes those four.

## Decisions

### 1. Duplication — **adopted, native, report-only**

`internal/duplication`, consumed by `regulus duplication`.

* **Method**: `go/parser` + `go/scanner`; identifiers and literals normalise to
  placeholders, then sliding windows of 40 normalised tokens are hashed. Two windows
  hashing equal are the same code modulo names and values (a Type-2 clone).
* **Why native**: it fits stdlib (`go/ast`, `go/scanner`, `go/token`, `hash/fnv`), it
  is deterministic, and it needs no build-config-aware load — so no dependency.
* **Why report-only**: a duplication budget has no oracle. A threshold would be an
  arbitrary number that fails builds for code a human might keep on purpose, which is
  the vanity-metric failure the phase excludes.
* **Limits, declared**: intra-function repetition is excluded on purpose (it is usually
  an intentional loop); `WINDOWS` counts overlapping windows, so it measures how much
  code two functions share, not how many distinct blocks they repeat; `_test.go` files
  are excluded by default because tests repeat fixtures deliberately.

| Candidate | License | Verdict |
|---|---|---|
| `dupl` (mibk) | MIT | **Rejected**: first external require, no capability the native check lacks at this scale |
| `golang.org/x/tools` clone helpers | BSD-3 | **Rejected**: same dependency cost, and it would pull a large tree for one analyser |
| `jscpd` | MIT | **Rejected**: Node runtime for a Go-only question |

### 2. Dead code — **evaluated, deferred, follow-up named**

* **Why not adopted now**: the cheap native versions are false-positive machines.
  Unexported-method reachability is decided by interface satisfaction, not by name;
  reflection, build tags, `//go:linkname` and generated files all defeat a name-mention
  scan. A detector that cries wolf on live code teaches people to ignore it.
* **Why not `x/tools/cmd/deadcode`**: it is the correct tool and it is still a
  dependency, and it needs a `packages.Load` with full build configuration — which
  means the first external require *and* a slower gate.
* **Follow-up (explicit, not dropped)**: adopt `deadcode` behind a **non-required
  full-profile** gate when the project accepts its first analysis dependency. Until
  then the gap is recorded here rather than filled with a bad check.

| Candidate | License | Verdict |
|---|---|---|
| `golang.org/x/tools/cmd/deadcode` | BSD-3 | **Deferred**: correct, but first dependency + full package load |
| native name-mention scan | — | **Rejected**: false positives on interfaces, reflection and build tags |
| `staticcheck` | MIT | **Rejected**: large analyser set for one check, and a dependency |
| `golangci-lint` | **GPL-3.0** | **Rejected**: license is incompatible with this project's distribution terms |

### 3. Vanity metrics — rejected on principle

LOC, comment ratio, Halstead volume, "technical debt ratio" and uniform coverage
targets. None has an oracle, none changes a decision, and the phase excludes them by
name. CRAP survives only because coverage and complexity feed a threshold that a human
chose and can defend.

## How to use this

```bash
regulus duplication                     # report over the repo
regulus duplication --min-tokens 60     # fewer, stronger findings
regulus duplication --json              # machine-readable
```

A finding is a question, not a verdict: read the pair, then decide whether the shared
code should be extracted, parameterised, or left alone because the two cases will
diverge.
