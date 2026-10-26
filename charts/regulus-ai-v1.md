# Regulus AI v1 — a complete harness built in phases

Founding feature: build Regulus AI v1 as a **complete harness** — Gauntlet,
Discipline and Evolution fully developed — as a personal product, native to
OpenCode V2. v1 is reached through phases: F1 ships a thin vertical spine,
F2–F4 complete each loop, F5 integrates and releases. This document is the
spec subagents read; handoffs reference it by task and spec, never by
paraphrase. Read down to `## Log` before working.

## Specs

- **S1**: "quiero crear un harness parecido a gentle ai que se use para
  opencode, pero con ciertas mejoras." — base:
  https://github.com/Gentleman-Programming/gentle-ai
- **S2**: "las mejoras que quiero implementar son las de harness2 … esto con
  el objetivo de llevarlo a un nivel completo." —
  https://github.com/google-research/harness2
- **S3**: "tambien quiero incluir las ideas del uncle Bob creador de clean
  code cuando menciono que no revisa el codigo que la IA genera, sino que
  tiene unos agentes encargados de velar por que el codigo sea de nivel."
- **S4**: founding decisions (user questionnaire, verbatim answers): v1
  scope = "Slice vertical de los 3 loops"; evolution = "Reflect + adopt con
  evidencia"; audience = "Producto personal"; core language request =
  "analisa tanto GO como Rust para ver cual de los dos es mejor" [sic], later
  resolved by the user selecting "Go core (recomendado)" after the analysis.
- **S5**: "todo debe ser para opencode v2, unicamente considerar este."
- **S6**: "Regulus AI debe ser un harness completo desde la v1, asi que
  debemos crear varias fases para ir llevando todo a esa v1 completa e
  increible"
- **S7**: "tambien debemos hacer algo parecido a engram de gentle ai, pero
  para esto debemos tomarlo como ejemplo y ver de que forma podemos
  mejorarlo para adaptarlo a regulus y que funcione de forma optima y
  precisa"

## Acceptance

### Scenario: doctor refuses non-V2 runtimes

- Given `opencode --version` reports V1, fails, or prints unparseable output
- When `regulus doctor` runs
- Then the exit code is 1 and the refusal names the failed condition and the V2 requirement

### Scenario: doctor accepts V2 runtimes

- Given `opencode --version` reports a 2.x version
- When `regulus doctor` runs
- Then the exit code is 0 and the output reports supported (V2)

### Scenario: spec check enforces the acceptance contract

- Given a feature document
- When `regulus spec check` runs on it
- Then the exit code is 0 with the scenario count, or 1 listing every violation with its line number

### Scenario: gates stop at the first failure and write receipts

- Given a fixture project with gates configured and one failing gate
- When `regulus gate run` executes
- Then the exit code is 1, no later gate runs, and an append-only receipt records the failure

### Scenario: reflect admits only evidenced lessons

- Given a closed feature with receipts on disk
- When `regulus reflect` runs
- Then lesson candidates carry cue, lesson, evidence and confidence, and the budget ledger records the run

### Scenario: session memory persists across restarts

- Given notes saved through `regulus memory note`
- When `regulus memory show` runs in a later session
- Then every note is listed with its timestamp, oldest first

## Tasks

| ID | Spec | Work | Route | Commit |
|---|---|---|---|---|
| T1 (F0) | S1–S6 | Foundation: PRD, architecture, README, this document, repo skeleton | inline (parent) | 4c135bf |
| T2 (F1) | S1–S6 | Vertical spine end-to-end: CLI skeleton (contract, doctor with V2 probe), spec contract + `spec check`, gate layers 1–4 with append-only receipts, first guardian agent, workflow skill, minimal session memory, minimal reflect + library + budget | inline (writer subagent unavailable) | a9cc94e · f78a132 · 9e17fb0 · 199caf3 · 3906b54 · this unit (WU6) |
| T3 (F2) | S1, S3, S5 | Complete Gauntlet: gate layers 5–7 (coverage floor, mutation testing, CRAP), full guardian roster, evidence audit UX, per-project gate thresholds | inline (writer subagent unavailable) | bb55401 · 3389543 · 2ae3bf6 · 5397b1c · a6491be · this unit (WU5) |
| T4 (F3) | S1 | Complete Discipline: the full Sextant workflow + task tooling, subagent roster with permission.task matrices, complete skills, structured session memory + cross-restart resume | inline (writer subagent unavailable) | 9b940f6 · 6e0eba8 · this unit (WU4) |
| T5 (F4) | S2 | Complete Evolution: lesson admission flow, library retrieval into context, guardian-validated surface edits under budget caps, opt-in probe cycle (k=1) | decide at task start | — |
| T6 (F5) | S1–S6 | Integration & v1 release: complete V2 plugin (all hooks + custom tools), complete CLI UX, journeys suite, npm installer, user docs, final dogfood | decide at task start | — |

## Log

- **L1** (original request, verbatim): "quiero crear un harness parecido a
  gentle ai que se use para opencode, pero con ciertas mejoras. este es el
  repo de gentle ai: https://github.com/Gentleman-Programming/gentle-ai las
  mejoras que quiero implementar son las de harness2:
  https://github.com/google-research/harness2 esto con el objetivo de
  llevarlo a un nivel completo. tambien quiero incluir las ideas del uncle
  Bob creador de clean code cuando menciono que no revisa el codigo que la IA
  genera, sino que tiene unos agentes encargados de velar por que el codigo
  sea de nivel. analiza estos puntos y vayamos definiendo como crearemos esta
  nueva herramienta llamada Regulus AI"
- **L2** (questionnaire decisions): "Slice vertical de los 3 loops" ·
  "Reflect + adopt con evidencia" · "Producto personal" · custom language
  answer "analiza tanto GO como Rust para ver cual de los dos es mejor".
- **L3** (Go-vs-Rust analysis, resolved): facts weighed — Go 1.27 stabilized
  `encoding/json/v2`; OpenCode V2 broke its V1 plugin API (surface churn);
  architectural decision that the TS plugin owns the typed OpenCode boundary
  while the core speaks the Regulus-owned contract. User selected: "Go core
  (recomendado)".
- **L4** (M0 evidence): PRD, architecture, README and this document committed
  in the initial commit of this repository. Design baselines: 8-component
  harness surface (Harness²), receipts as append-only JSON, gate layering
  cheap→expensive (Uncle Bob), budget-gated evolution.
- **L5** (requirement change, verbatim): "1. todo debe ser para opencode v2,
  unicamente considerar este. 2. Regulus AI debe ser un harness completo
  desde la v1, asi que debemos crear varias fases para ir llevando todo a
  esa v1 completa e increible"
- **L6** (roadmap revision): v1 redefined as a complete harness reached
  through phases F1–F5 (F1 = thin vertical spine of the three loops — the
  former M1–M4 compressed; F2–F4 complete each loop; F5 integrates and
  releases). OpenCode V2 only — no V1 compatibility layer. Evolution default
  unchanged (reflect + adopt with evidence, per L2); Propose–Probe–Compose
  enters in F4 as an opt-in k=1 probe cycle behind the budget ledger. PRD §4
  and this document rewritten in the roadmap revision commit.
- **L7** (F1/WU1 evidence): Go module `regulus-ai` (go 1.27); CLI skeleton
  with help/--version/contract/doctor; doctor probes `opencode --version`
  with a 15s budget and refuses V1/missing/unparseable/failing runtimes,
  including the runtime's own output in the refusal — OpenCode V2 only
  (S5). RED observed (14 failing assertions on stubs), then GREEN; go vet
  and gofmt clean; smoke on the real environment took the refusal path
  (the local opencode.exe is currently broken on this machine). Commit:
  a9cc94e.
- **L8** (F1/WU2 evidence): spec contract live — `regulus spec check`
  validates the Acceptance section (Given/When/Then, And-continuation,
  line-numbered violations, all at once). RED observed (15 failing
  assertions: 11 spec package + 4 CLI), then GREEN. This feature document
  now declares F1's acceptance criteria and passes `regulus spec check`
  (dogfood). Commit: f78a132.
- **L9** (F1/WU3 evidence): gauntlet runner live — `regulus gate run <task>`
  loads the committed `regulus.json`, runs gates in ascending layer order
  (cheap first), stops at the first failure, and writes one append-only
  receipt per executed gate under `.regulus/evidence/<slug>/` (bounded
  stdout/stderr with SHA-256 of the full content, git HEAD, duration;
  O_EXCL — never overwritten). Doctor output fixed to print the runtime's
  own version line exactly. RED observed (25 failing assertions across
  receipts/gates/cli), then GREEN; go vet and gofmt clean. Dogfood:
  `regulus gate run regulus-ai-v1/T2` on this repo passes 4/4 gates with
  receipts (git_head f78a132); the stop-at-failure demo ran only the
  failing layer. Commit: 9e17fb0.
- **L10** (S7 evidence): Almanac memory design written (docs/memory.md) —
  Engram studied from Gentle-AI's own docs and deliberately improved:
  provenance-first observations, append-only revision chains (no silent
  upserts), deterministic retrieval by construction (project → topic
  key/domain bucket → exact match; no embeddings in v1), staleness
  annotations, one substrate with typed views (unifying Engram session
  memory with Harness² lesson libraries), data-not-injections framing,
  and code-enforced lesson admission. F1 ships the session-notes slice
  (`regulus memory note/show`); F3 ships the full substrate (PRD §4 F3
  updated). Acceptance scenario 6 added. Commit: 01a6fa5.
- **L11** (F1/WU4 evidence): OpenCode boundary live — V2 plugin at
  `.opencode/plugins/regulus.ts` registers the regulus tool namespace
  (gate_run, spec_check, doctor, memory_note, memory_show), two slash
  commands, a context hook carrying the Regulus discipline, and a
  compaction hook injecting `regulus memory show` as a DATA block
  (evidence-is-data invariant). Fail-fast version gate: setup resolves
  a regulus binary (REGULUS_BIN → bin/regulus.exe → bin/regulus → PATH)
  and refuses to load without a doctor-verified V2 runtime. Guardian
  agent regulus-gate-verifier (read-only permissions, bash allowlist)
  and the regulus-workflow skill ship as `.opencode/` assets. Go:
  memory package (session notes) — RED 5 → GREEN, vet/fmt clean, live
  smoke (note+show on the real binary). TS: `tsc --noEmit` clean against
  @opencode/plugin 2.1.4 (Bun 1.4.4 installed). Live plugin-load
  verification happens in the user's OpenCode session (WU6/F5 dogfood).
  Commit: 199caf3.
- **L12** (F1/WU5 evidence): Evolution slice live — `regulus reflect`
  validates the feature document's spec contract, requires the task's
  receipts (refuses without evidence), admits a lesson with
  cue/lesson/evidence/confidence and records the budget entry;
  `regulus library list` reads per-domain INDEX views; `regulus budget
  show` prints the ledger. RED 13 → GREEN, vet/fmt clean. Dogfood:
  reflect over this feature document citing the 4 real WU3 gauntlet
  receipts — lesson les_18e2043c388e0260 admitted (medium confidence),
  INDEX and budget entry written. Commit: 3906b54.
- **L13** (F1/WU6 evidence — PHASE EXIT): the deterministic journey
  `TestF1VerticalSpine` (internal/journey) drives a fixture project
  through the whole spine in-process with real shell gates: doctor (V2
  gate) → spec check → gate run leaving 2 append-only receipts → memory
  note/show → reflect admitting an evidence-gated lesson → library
  list → budget show. The journey's first run caught one integration
  gap — an assertion expecting the lesson text where `library list`
  prints the cue (the pinned CLI contract) — corrected, then GREEN
  with the full suite; vet and gofmt clean. F1 exit criterion met: a
  tiny feature developed through Regulus produces receipts for all
  three loops. Live plugin-load verification remains for the user's
  OpenCode session (F5 dogfood). Commit: 3808f1b.
- **L14** (F2/WU1 evidence): metric gates live — a gate may declare
  {extract: last-line-float|last-line-percent, threshold, operator:
  >=|<=}; the command must exit 0 AND the extracted value must satisfy
  the operator (extraction: last non-empty stdout line, percent strips
  "%", last numeric token wins; unextractable = gate failure with the
  refusal in the receipt stderr). Receipts carry metric_value,
  metric_threshold and metric_operator when evaluated. Config validation
  rejects bad operators and extract modes. Gremlins installed for the
  mutation gate (exit-code thresholds built in). RED 5 → GREEN, suite /
  vet / fmt clean. Commit: bb55401.
- **L15** (F2/WU2 evidence): CRAP analyzer live — `regulus crap --profile
  <go cover profile> [--max N] [--top N]` parses the profile (mode header
  + block lines), computes per-function cyclomatic complexity via go/ast
  (1 + branch statements + short-circuit operators; nested function
  literals count toward their parent), maps profile blocks onto functions
  for coverage, and applies Uncle Bob's formula comp²×(1-cov)³+comp;
  exit 1 when max CRAP exceeds the threshold (default 30). RED 6 → GREEN
  (the RED run caught an arithmetic slip in the formula test's own
  expected value: CRAP(10,0) = 110, not 1010 — fixed; the implementation
  was right). First live run ON REGULUS ITSELF found real debt:
  CLI.libraryCmd CRAP 41.20 (complexity 11, coverage 37%), spec.Check
  30.18 (complexity 30), internal/crap.exprName and gates.ShRunner.Run
  uncovered — the layer-7 gate refused (exit 1). Paying this debt (tests
  for the uncovered paths; spec.Check decomposed below complexity 30)
  is part of the F2 exit criterion. Commit: 3389543.
- **L16** (F2/WU3 evidence): the human audit surface is live — `regulus
  evidence list [--task <t>]` prints one line per receipt (timestamp,
  layer, gate, verdict, exit code, duration, metric, path) and
  `regulus evidence show <file>` renders one receipt in full (id, task,
  gate, command, verdict, timing, metric, git_head, sha256-pinned
  stdout/stderr). receipts.ReadFile parses one append-only receipt;
  internal/evidence.List walks the store path-ordered with a task
  filter and refuses malformed receipts. RED 8 → GREEN, suite / vet /
  fmt clean; live demo over the real WU3 gauntlet receipts of
  regulus-ai-v1/T2. Commit: 2ae3bf6.
- **L17** (F2/WU4 evidence): the seven-layer gauntlet is green on regulus
  itself — `regulus gate run regulus-ai-v1/T2` exits 0 with 7/7:
  typecheck, lint, build, unit-tests, coverage-floor (86.7% ≥ 80,
  metric gate), mutation (gremlins efficacy ≥ 70 over a clean
  git-archive export of HEAD — 84.62% calibrated in 13s; the naive
  whole-repo temp copy starved every mutant to timeout on Windows
  because gremlins copies the module per mutant, node_modules
  included), CRAP (max 21.23 ≤ 30). Debt from L15 paid first (commit
  5397b1c): libraryCmd/reflect paths covered (coverage 81.1→86.7%),
  spec.Check decomposed 30→15, a REAL Windows bug fixed (ShRunner
  deadline kill surfaced as ExitError exit-1, masking timeouts —
  context check now precedes the ExitError branch), receiver names
  render like Go tooling ((*calc).Sub). Guardian roster complete:
  spec-keeper, mutation-auditor, crap-analyst, evidence-auditor,
  gate-verifier leading with metrics across all seven layers.
  Commit: a6491be.
- **L18** (F2/WU5 evidence — PHASE EXIT): the deterministic journey
  `TestF2SevenLayers` (internal/journey/f2_test.go) walks a REAL Go
  fixture module (mutation-killing tests, git init + commit) through
  the complete seven-layer gauntlet in-process with a freshly built
  regulus binary: 7/7 gates pass in ~12s, 7 receipts written, the
  coverage-floor receipt records its metric, mutation and CRAP receipts
  prove pass. The journey skips with a printed reason when gremlins is
  absent (the mutation gate was verified live in the L17 dogfood).
  Building the gates config via json.Marshal of the typed gates.Config
  caught a JSON-escaping trap a hand-written template hid. F2 exit
  criterion met: the full 7-layer gauntlet runs green on a real repo,
  with receipts the human audits without opening a diff.
  Commit: 3792dc3.
- **L19** (nomenclature decision, user verbatim): "podemos cambiar ODD
  y crear nuestra propia nomenclatura para evitar robar la idea, sino
  adaptar a nuestra idea" — resolved as **The Regulus Method**. Our own
  vocabulary replaces every borrowed name in live text: the workflow is
  the Regulus Method (five movements: authorize → chart → build →
  prove → learn); feature documents are Charts under `charts/` (was
  `odd/tasks/`, moved with git mv preserving history); the Log keeps
  its name (the ship's log); the future subagent roster will be the
  Crew. Attribution stays where it belongs: the sources (PRD §1) and
  README still credit Gentle-AI, Harness², Engram and Uncle Bob as
  inspirations — ideas are adapted under their MIT/Apache-2.0 terms
  and credited; only the borrowed NAMES are retired from Regulus's own
  vocabulary. Historical Log entries stay verbatim as the audit
  trail. Commit: 2616f25.
- **L20** (nomenclature revision, user verbatim): "podemos cambiar el
  nombre de the Regulus method por algo que haga referencia a
  Astrolabio, sextante, etc. algo mas estelar o cosmico" — resolved
  as **The Sextant** (El Sextante). The method that runs the loops is
  named for the stellar measurement instrument: it takes the sighting
  (the receipt), the Log records it, the Chart plots it, the Almanac
  remembers it — the navigation metaphor closes. "The Regulus Method"
  (L19) is retired from live text; the five movements are unchanged
  (authorize → chart → build → prove → learn). Commit: b3e23ee.
- **L21** (F3 evidence — PHASE EXIT): the Almanac is the substrate
  (WU1+2, 9b940f6): `.regulus/almanac/observations.jsonl` — append-only,
  typed, provenance first-class; `memory note|show` rebacked on it
  (internal/memory absorbed, CLI surface unchanged);
  `save|search|history|context` with deterministic bucket-then-match
  retrieval, age annotations and the resume block framed as DATA;
  one-substrate unification — reflect also appends the lesson
  observation (LESSON.md stays the human view); lesson-type saves
  refused at the CLI (one door: reflect, evidence-gated). The Crew
  landed (WU3, 6e0eba8): regulus-scout (read-only mapping with
  path:line evidence), regulus-worker (one bounded unit against a
  Chart, hands back receipts), regulus-verifier (independent
  reproduction); plugin tools memory_save/search/context (tsc
  clean) and the compaction hook now injects the full Almanac
  context. Exit criterion proven by the TestF3Resume journey (WU4):
  after a cold restart, the Chart plus `regulus memory context`
  surface the chart pointer, the in-flight notes and the saved
  decisions — a multi-session feature resumes from the feature
  document + Almanac alone. Commit: this work unit.
