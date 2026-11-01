# docs-ci-hygiene — docs veraces, CI acotado, init con floors, tests robustos

User authorization: plan total 2026-10-31, cuarto = P2 docs/CI/tests. Read down to `## Log` before working.

Class: COMPLEX

Domain: sextant

## Contract

- Objective: docs que coinciden con el código, CI acotado y moderno, scaffolds con floors, tests que no castigan refactors.
- Scope: `docs/*.md` (stale), `.github/workflows/gauntlet.yml`, `internal/cli/init.go`, tests TS source-assertion + journey mutación. Sin cambiar código prod (salvo init template).
- Constraints: solo docs/CI/template/tests; pins de toolchain intactos; `bun test` + `go test` verdes.
- Acceptance: S1, S2, S3, S4
- Artifacts: receipts `docs-ci-hygiene/C*`.
- Dependencies: C1–C4 independientes.
- Risk: STANDARD-equivalente por unidad. Class COMPLEX por multi-unidad.
- Verification: `spec check` + `gate run docs-ci-hygiene/C*`.
- Completion: 4 tasks CLOSED, `spec OK: 4`.

## Acceptance

### Scenario: S1 docs veraces

- Given `architecture.md` (7 gates, `contract --version`), `memory.md` §2/§5, snapshots viejos, `gauntlet.md` schema
- When se corrigen contra el código
- Then 12 gates, `contract` sin flag, schema `category/required/...`, memoria v2 vigente, snapshots marcados históricos o actualizados

### Scenario: S2 CI acotado

- Given job sin `timeout-minutes`, `ubuntu-latest` flotante, actions Node20
- When se fija `timeout-minutes`, pin `ubuntu-24.04`, actions con Node24
- Then CI corre igual y verde, sin warnings Node20

### Scenario: S3 init con floors

- Given `init --stack go|node|smoke` genera gates mínimos sin floors
- When se amplía el template (coverage/crap/spec/gofmt donde aplique, allowlist, evolution block) y `--harness-from` copia `sync-plugin-defaults.ts`
- Then proyecto nuevo nace con floors y codegen completo

### Scenario: S4 tests robustos

- Given source-assertions en `domain.test.ts` y pin `files.length==11`
- When se convierten a behavior (o se documentan intencionales con helper de registro)
- Then refactors puros (renombrar) no rompen tests y `bun test` verde

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| C1 | S1 | docs stale fix | @regulus-worker |
| C2 | S2 | CI timeout-minutes + ubuntu pin + Node24 actions | @regulus-worker |
| C3 | S3 | init floors + harness-from sync script | @regulus-worker |
| C4 | S4 | tests behavior (o registro intencional) | @regulus-worker |

## Log

- L1: Chart opened 2026-10-31 (plan total, P2). Scout TS `ses_e6ba201aaffee5hldG8jzvjMKv` §4/§5/§7/§2: arch/mem/gauntlet stale; CI sin timeout + Node20 + latest; init sin floors (`init.go:40-191`); source-assertions (`domain.test.ts:218-540`).
- L2: C1–C4 CLOSED 2026-11-01 — C1 docs veraces (`ses_e6b6c3b5cffeNOwPovVYhsCBR1`, 12/12), C2 CI acotado (`ses_e6b6c3b54ffeFxqWXMKLv2Z1mX`, timeout 30 + ubuntu-24.04 + checkout@v5/setup-go@v6, upload-artifact@v4 se queda con causa, actionlint 0, 12/12), C3 init floors + harness-from sync (`ses_e6b6c3b54ffdnX8kdVs4uzEf34`, 12/12), C4 behavior (`ses_e6b6c3b53ffe0lISgLkMOysXIM`, bun 76/0, refactor-puro verde, 12/12). `spec OK: 4`. Cierre manual.
