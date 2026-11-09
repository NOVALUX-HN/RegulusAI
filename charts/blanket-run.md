# blanket-run — blanket "termina todo" funciona sin parar por tarea

User authorization: "es correcto. procede con el plan y avisame hasta que finalices todo" 2026-11-03. Blanket para B1→B2→O1→O2→T/V/P→E. Solo notificar al final o ante bloqueo rojo humano. Read down to `## Log` before working.

Class: STANDARD

## Contract

- Objective: que "termina todas y avísame al final" encadene sin `question` intermedia; intermedios a `memory_note`, aviso solo al final o bloqueo rojo.
- Scope: `.opencode/discipline.md` + `.opencode/agents/regulus.md` (regla blanket 2-3 líneas), `memory_save topic_key=modo-trabajo` ya existente. Sin tocar `opencode.json`, `regulus.json#allowlist`, gates, `yolo.go`, ni habilitar `webfetch`.
- Constraints: no cambiar denies destructivos ni guardians read-only; no cambiar scope/contract de otros Charts; frase canónica exacta "termina todo y avisame" como trigger; `spec check` OK.
- Acceptance: B1, B2
- Artifacts: receipts `blanket-run/B*` (12/12 por tarea).
- Dependencies: B1 antes que B2 (B2 prueba la regla de B1).
- Risk: standard
- Verification: `spec check charts/blanket-run.md` + `gate run blanket-run/B*` 12/12 + prueba viva O1→O2 sin `question`.
- Completion: B1–B2 CLOSED vía `task close`.

## Acceptance

### Scenario: B1

- Given `.opencode/discipline.md:8-13` y `.opencode/agents/regulus.md:47-49` hoy dicen `ONE question and stop` sin concepto blanket persistente
- When se añade regla blanket (no `question` hasta último Oi o bloqueo rojo humano; intermedios a `memory_note`; `doom_loop/task` pre-autorizados solo en blanket, denies vigentes)
- Then `grep -i blanket .opencode/discipline.md` no vacío, `spec check` da `spec OK`, y `grep doom_loop opencode.json` sigue `ask`

### Scenario: B2

- Given B1 aplica y `charts/obscura-default-search.md` tiene O1/O2 `open` con `spec OK:2`
- When se encadenan O1→O2 sin `question` intermedia registrando intermedios en `memory_note`
- Then ambos Oi llegan a `gate_run 12/12` o a bloqueo rojo documentado, sin parada por autorización intermedia

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| B1 | B1 | discipline+agent: regla blanket 2-3 líneas | @regulus-worker | done |
| B2 | B2 | prueba viva encadenar O1→O2 sin question | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-03 desde scout `ses_e606bfce8ffe8lm7oZPmaDddhW` (doom_loop ask, yolo solo bash/edit + reload, worker task:deny, halt al primer rojo, nota blanket es DATA). Blanket autorizado para noche completa. O1/O2 ya chartados spec OK:2.
- L2: B1 implementada 2026-11-03: regla blanket en `.opencode/discipline.md` (+0 líneas netas, 13→13, núcleo sin backticks por codegen) y `.opencode/agents/regulus.md` (zona AUTHORIZE +2). `opencode.json` intacto (doom_loop ask), sin tocar `regulus.json`/`internal`/gates/`yolo.go`, `webfetch` sigue deny. `sync-plugin-defaults --write` regeneró `.opencode/plugins/regulus.ts`. Gate completo en curso.

- CLOSED B1 STANDARD spec OK + 12 receipt(s) PASS (run_bacaeb751be73c10).
- L3: B2 prueba viva 2026-11-03: B1 vigente verificado (`discipline.md:12-13` + `regulus.md:50-51` con blanket, `opencode.json:23` doom_loop ask, `spec OK: 2` blanket-run + `spec OK: 2` obscura-default-search con O1/O2 open). Cero `question` emitida; intermedios solo a `memory_note` (2 notas). `gate_run blanket-run/B2` 12/12 PASS (run_828ba0151988465e). Sin cambios Go/permisos/gates. La cadena O1→O2 puede continuar sin parada por autorización intermedia.
- CLOSED B2 STANDARD spec OK + 12 receipt(s) PASS (run_828ba0151988465e).