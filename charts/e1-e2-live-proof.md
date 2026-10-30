# E1-E2 live proof — cierre harness2-light en sesión viva

User authorization: "hazlo" — cerrar E1/E2 pendientes de v0.6-elite (S3 live re-proof, S4 trace→domain loop). Read down to `## Log` before working.

Domain: sextant

## Acceptance

### Scenario: live boundary re-proof in real OpenCode

- Given OpenCode V2 runtime en esta máquina y este repo
- When `regulus doctor` corre y la sesión viva llama herramientas regulus (skills_list, memory_context, trace_sessions)
- Then doctor reporta supported V2, `trace sessions` lista esta sesión viva, `trace show` imprime fingerprint y `trace files` lista touched paths con `.regulus/traces/<session>.jsonl` en disco

### Scenario: traces to domain to lessons end-to-end

- Given una sesión viva con traces y receipts de un task con evidencia
- When corre `evolve propose --task <t> --domain <d>` y `probe contrast` sobre un gate real
- Then el pack trae fingerprints + receipts + INDEX + decisions etiquetados, y el contraste A/B devuelve diff de receipts; el loop cierra con lección admitida o candidato con evidencia citada

### Scenario: gauntlet green cierra el chart

- Given el árbol con este chart
- When corre `gate run e1-e2-live-proof/E1` (o task equivalente)
- Then todas las capas configuradas pasan y cada capa deja receipt append-only

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| E1 | S3 v0.6-elite (F8 checklist 1-4) | live re-proof: doctor + disciplina inyectada + traces vivos + memory_search dimension/path | @regulus-worker |
| E2 | S4 v0.6-elite (F8 checklist 5-6) | live trace→domain loop: evolve propose + probe contrast + admisión o candidato evidenciado | @regulus-worker |

## Log

- L1: Chart opened desde "hazlo" — E1/E2 eran los únicos `[ ]` en v0.6-elite:26-27, bloqueados en "owner's live OpenCode session". Esta sesión viva ES esa sesión.
- L2: E1 CLOSED — Task E1 "live re-proof: doctor + disciplina inyectada + traces vivos + memory_search dimension/path" Spec S3. doctor `opencode v2.2.3 — supported (V2)` (diverge de v2.2.0 esperada, más nuevo); disciplina + suggested_skills + admitted_lessons inyectados como DATA; skills_list + memory_context vivos; memory_search dimension/path operativos (vacío lexical-only); trace sessions 31 incl. viva, show fingerprint OK, files OK, `.regulus/traces/<session>.jsonl` en disco.
- L3: E2 CLOSED — Task E2 "live trace→domain loop: evolve propose + probe contrast + admisión o candidato evidenciado" Spec S4. evolve propose ci-green/C2 trae 4 secciones etiquetadas + (none); probe contrast spec OK 4 vs 6 con diff; probe compose (no conflicts) 5 receipts probe/; sin candidatos pendientes, les_18e240ab097696d0 ya admitted — loop cierra por rama admitida. Binary gate E2 9/9 (typecheck/lint/build/unit-tests/coverage 83.7/mutation 83.33/crap/vulncheck/spec). Divergencias D1 stamp unstamped, D2 semantic stale, D3 MCP gate_run null (binario sí 9/9), D4 glob hidden-dir + trace_files inconsistente. Chart CERRADO.
