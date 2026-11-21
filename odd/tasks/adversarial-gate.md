# Feature: adversarial gate (deterministic operators)

New first-class gate `adversarial run`: runs the pinned catalog + deterministic operator expansion, reports containment rate, fails below threshold. Generation without a model so verdicts stay reproducible; model proposals enter later via evolve.

## Specs

- S1: "me gustaria agregar un gate nuevo de test tal como estan los otros" — new gate wired exactly like the others: CLI command + `regulus.json` entry (layer, command, metric) + receipts per run.
- S2: "pero este es especial porque quiero que se oriente a poder crear test adversariales" — the gate CREATES adversarial cases: deterministic operators (case-fold, homoglyph, traversal, separators, oversize, corrupt-jsonl, timeout edges, control chars) applied to catalog seeds, evaluated by per-component containment oracles; escaped variants are reported as catalog candidates, never auto-fixed.
- S3: "cual recomeindas para que pueda ser parte del flujo y que nos aporte mejora en la calidad del software que hagamos" — decision: deterministic operators (reproducible verdicts, measurable containment-rate); model-assisted proposals stay out of the gate and enter via evolve/learn.
- S4: verdict rule — REQUIRED gate (same rigor as unit-tests/crap: runs in every profile, blocks on fail), catalog entries must all contain (100%); expansion metric `contained/total` with `--min-containment` default 100 (tunable down only with reason); last line prints `containment: P% (C/N)` for the standard percent extractor; exit non-zero below threshold. Layer placement: inside the test band next to crap if no test/doc pins layer numbers, else appended as next free layer (uniqueness rule) — assessed in T3.

## Tasks

- T1 (S1,S4): `adversarial run` CLI + catalog loader + runner with per-entry verdicts and last-line metric. Route: inline. Commit: (pending)
- T2 (S2): operator library (pure, golden-pinned) + per-component oracles reusing gates/evidence/receipts dry checks. Route: inline. Commit: (pending)
- T3 (S1): `regulus.json` gate entry (layer + command + metric) and layer placement per validation rules. Route: inline. Commit: (pending)
- T4 (S1,S4): update pinned goldens asserting gate counts (bench B5/B9/B13 + any docs listing 23 gates). Route: inline. Commit: (pending)
- T5 (S2,S3): docs: catalog rule extension (expansion candidates → evolve admission). Route: inline. Commit: (pending)

## Log

- L1 (user, verbatim): "me gustaria agregar un gate nuevo de test tal como estan los otros, pero este es especial porque quiero que se oriente a poder crear test adversariales."
- L2 (user, verbatim): "investiga sobre estos test y analiza como estos se pueden complementar con el funcioonamiento de regulus"
- L3 (user, verbatim): "cual recomeindas para que pueda ser parte del flujo y que nos aporte mejora en la calidad del software que hagamos"
- L5 (user, verbatim): "pero quiero que este este al mismo nivel que los test unitarios, test de mutacion, etc que ya tenemos para poder verificar que nuestro codigo este al 100. quiero que estos test complementen el flujo de forma precisa"
- L6 (decision): required + blocking + default threshold 100. Same rigor as unit-tests/crap, not optional like mutation/vulncheck. Any future escape fails the build until fixed or admitted as a catalog entry via evolve.
- L7 (scope pause): user asked for analysis-only before building ("ahorita solo analiza si podemos implementarlo y cual seria la mejor forma"). T1-T5 below remain the build plan, untouched.
- L8 (user, verbatim): "analiza esa documentacion y mejoremos el plan de lo que queremos hacer" (re: linesncircles adversarial-AI-testing, aisecurityandsafety adversarial-robustness, eesel adversarial-testing). Refinements R1-R6 below adopted from that round; model-side tooling (ART/Foolbox/TextAttack/PyRIT/Garak) explicitly OUT of the gate (needs differentiable models or LLM judges) — belongs to evolve proposals at most.

## Refinements from adversarial-AI guides (R1-R6)

- R1 (no-go registry + standards mapping): catalog entries and operators map to explicit invariants (never-exec-unlisted, never-write-outside-jail, never-store-untruncated-hostile, never-leak-secrets-into-receipts) with refs where fitting (OWASP LLM01 prompt-injection framing, LLM04 provenance poisoning, MITRE ATLAS technique). Implement as optional `standard` + `severity` fields per entry (catalog stays v1, existing pin test untouched) plus an invariants doc section.
- R2 (dual metric — precision guard): containment on hostile set AND pass-through on a new benign-tricky seed set (legit edge cases that MUST pass: dotted task ids, long-but-valid commands). Pass-through must be 100%: blocks deny-everything gaming. Gate reports both; threshold applies to containment.
- R3 (new operator classes from the guides): (a) provenance attacks — tampered-sha receipt, oversized fields, future-dated runs (oracle: `evidence verify`/list must refuse or flag); (b) exfiltration probes — secret values smuggled in commands (oracle: captured stdout/stderr must not contain them, exercises the existing SecretValues redaction); (c) implicit/benign-tricky seeds for the R2 pass-through set.
- R4 (severity + breakdown): per-entry `severity` (critical/standard, matching repo vocabulary); report shows per-severity containment so escapes concentrate visibly; global threshold stays simple.
- R5 (benchmark artifact): every run writes versioned `adversarial-report-v1` JSON to evidence (catalog+operators versions, per-entry/per-operator matrix, both metrics, commit) — trendable across releases like bench compare.
- R6 (candidate report for humans): each escaped variant reports MITRE/OWASP mapping + suggested seed bytes + failing invariant, so guardian/human admission via evolve is fast. Eesel-style human-review loop stays OUTSIDE the gate (admission), never inside the verdict.
