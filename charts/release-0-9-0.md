# release-0-9-0 — merge + versión

User authorization: "haz el merge y sube la versión" — tras delivery `c599145` (health-gaps 12 gates + live-plus-gaps + polish-gaps + almanac robustez + g1-g4 + doctor V2). Read down to `## Log` before working.

Domain: sextant

## Acceptance

### Scenario: versión 0.9.0 consistente

- Given `package.json` en 0.8.1 y el árbol en `c599145` verde 12/12
- When se sube a 0.9.0
- Then `package.json`, `README.md` status, `CHANGELOG.md` entrada 0.9.0, `bin/regulus` + `binaries/` stampleados 0.9.0 y `doctor` reporta `matches package.json`

### Scenario: release gauntlet verde

- Given el árbol con versión 0.9.0
- When corre `bin/regulus gate run release-0-9-0/R1`
- Then 12/12 pass con receipts append-only

### Scenario: merge a main + tag

- Given `feat/v0.4-evolution` verde y `origin/main` en `6ca6452`
- When se mergea a `main` y se taguea `v0.9.0`
- Then `main` avanza sin conflictos, el push incluye tag, y CI queda en verde

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| V1 | S1–S2 | bump 0.9.0 (package.json, README, CHANGELOG) + rebuild stampleado + release gauntlet | @regulus-worker |
| V2 | S3 | merge a main + tag v0.9.0 + push (+CI watch) | lead |

## Log

- L1: Chart opened 2026-10-30 desde "haz el merge y sube la versión". Convención: tags `vMAJOR.MINOR` en main al cerrar chart; bump en rama + rebuild + gauntlet, tag en main al merge. HEAD rama `c599145`, main `6ca6452`.
- L2: V1 CLOSED 2026-10-30 — bump 0.9.0 (package.json, README status, CHANGELOG `## [0.9.0]` + 12 receipts), rebuild `npm run build` (6 binaries + bin/regulus.exe stampleados 0.9.0, `doctor` → `matches package.json`), gauntlet `release-0-9-0/V1` 12/12. Sin tocar V2 (merge/tag: lead).
- L3: V3 CLOSED 2026-10-30 — ts-unit-tests rojo en main (`a249b29`, 49 pass/7 fail) → verde 56/56. Causa: checkout Windows CRLF vs marcadores estrictos LF en tests. Fix portable sin tocar Go: frontmatter `/^---\r?\n([\s\S]*?)\r?\n---\r?\n/` en `.opencode/lib/devin-pack.test.ts` + `.opencode/lib/yolo-permissions.test.ts` (2 sitios), fallback terminador CRLF en `.opencode/lib/domain.test.ts` + `scripts/sync-plugin-defaults.ts`; `gofmt -w internal cmd` solo normalizo CRLF→LF (92/92 Go identicos al blob, cero cambios Go). Gauntlet `release-0-9-0/V3` 12/12, evidencias en `.regulus/evidence/release-0-9-0__V3/` (ts-unit-tests `rec_18e3631f33a97de0`, gofmt `rec_18e3631f85774148`).
