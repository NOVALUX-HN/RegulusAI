# live-plus-gaps — prueba total v2 + aprendizaje harness2

User authorization: "Live + gaps (Recomendado)" — respuesta a "¿Qué significa 'actualiza todo y prueba flujos + aprendizaje harness2' en esta sesión viva?" (primero live proof, luego gaps que salgan en rojo). Read down to `## Log` before working.

Domain: sextant

## Acceptance

### Scenario: frontera viva V2 re-probada

- Given OpenCode V2 runtime en esta máquina y este repo
- When `regulus doctor` corre y la sesión viva llama `skills_list`, `memory_context`, `trace_sessions/show/files`
- Then doctor reporta `supported V2` + `matches package.json`, traces listan la sesión viva con fingerprint y touched paths en `.regulus/traces/<session>.jsonl`

### Scenario: gauntlet 12/12 verde en local

- Given el árbol con `regulus.json` de 12 gates
- When corre `bin/regulus gate run live-plus-gaps/L2`
- Then 12/12 pass con receipts append-only por capa

### Scenario: loop aprendizaje harness2 cierra en vivo

- Given una sesión viva con traces y receipts de L2
- When corre `evolve propose --task live-plus-gaps/L2 --domain go` y `probe contrast` sobre un gate real
- Then el pack trae fingerprints + receipts + INDEX + decisions, el contraste devuelve diff de receipts, y el loop cierra con lección admitida o `refused-cap`/sin-candidato explicado

### Scenario: gaps rojos quedan fijados o documentados

- Given los 7 gaps del scout (MCP recortado, stamp dev, stale, cap implícito, proposer estático, resolveDomain frágil, timeout largo)
- When el live proof los ejerce
- Then cada gap que salga en rojo queda fijado en el árbol o documentado con razón explícita en el Log; ninguno queda en silencio

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| L1 | S1 | live re-proof: doctor + skills + memory + traces vivos | @regulus-worker |
| L2 | S2 | rebuild stampleado + `gate run live-plus-gaps/L2` 12/12 + receipts | @regulus-worker |
| L3 | S3–S4 | evolve pack + probe contrast live + cerrar gaps rojos (MCP/stamp/stale/cap) o documentar | @regulus-worker |

## Log

- L1: Chart opened 2026-10-30 desde "Live + gaps (Recomendado)". Scout: 16 flujos en `internal/cli/* + regulus.ts + domain.ts`, aprendizaje surface/library/traces/ledger/reflect/INDEX/budget sin proposer autónomo; 12 gates, 15 charts, E1/E2 cerrados, ledger hasta H4-verify; riesgos stamp dev, stale, MCP recortado.
- L1 CLOSED 2026-10-30 S1 live re-proof vivo: doctor `version: 0.8.1 (matches package.json)` + `opencode v2.2.3 — supported (V2)`; `skills_list` regulus-workflow; `memory_context` 16 charts + notes E1/E2+D3+yolo+health; `trace_sessions` lista sesion viva `ses_e72699a6affeUoKCFQBZFUSMUZ`; `trace_show` fingerprint 31/31 completed sin errores; `trace_files` incluye `charts/live-plus-gaps.md` + `.regulus/traces/<session>.jsonl` verificado en disco; `spec_check` 4 scenarios OK. Sin `gate run` en L1: L1 es solo live sin codigo, los 12 gates son S2/L2. Gaps para L3 sin tocar: stamp ausente (`.regulus/stamp.json` no existe), stale (aviso `semantic index stale or missing — lexical only`), MCP recortado (`list_mcp_resources` 0 resources/0 templates).
- L2 CLOSED 2026-10-30 S2 rebuild stampleado + gate 12/12: `npm run build` (scripts/build.bash; seis `binaries/` + `bin/regulus` local) → `doctor` `version: 0.8.1 (matches package.json)` + `opencode v2.2.3 — supported (V2)`, sin WARNING dev; `regulus.json` 12 gates y workflow gauntlet 12 layers leídos sin deriva (sin cambios). `bin/regulus gate run live-plus-gaps/L2` 12/12 pass (typecheck 1080ms, lint 506ms, build 658ms, unit-tests 89892ms, coverage-floor 1043ms metric 83.80>=80, mutation 50090ms, crap 690ms, vulncheck 4281ms, spec 809ms, ts-typecheck 902ms, ts-unit-tests 3132ms, gofmt 1989ms). Receipts en `.regulus/evidence/live-plus-gaps__L2/`: typecheck-1793376976101262400, lint-1793376977189118100, build-1793376977714608700, unit-tests-1793376978380579200, coverage-floor-1793377076870177200, mutation-1793377036350520200, crap-1793377086441149300, vulncheck-1793377087133255600, spec-1793377091414754800, ts-typecheck-1793377092225138100, ts-unit-tests-1793377093127519200, gofmt-1793377096261218900. Ningún gate en rojo: nada que fijar. Sin tocar L1/L3.
- L3 CLOSED 2026-10-30 S3–S4 evolve pack + probe contrast live + gaps: `evolve propose --task live-plus-gaps/L2 --domain go` pack vivo con fingerprints (3 sesiones) + 24 receipts pass + INDEX go (1 lesson les_18e2043c388e0260) + decisions (none) + template; `probe contrast --task live-plus-gaps/L2 --gate spec --a spec-check-single vs --b spec-check-dir` diff vivo `spec OK: 4 vs 70 in 16` + probe receipts spec-1793377197194755900/spec-1793377197578525200 + budget probe-run sha256 605f6eed73357d06c6a3094137908e837974d976339d382e36b0eef1b4107ef3; re-propose muestra Probe evidence 2 receipts + Recent decisions probe-run (loop cierra sin-candidato: pack sin señal nueva, sin lección admitida ni refused-cap). Gaps: MCP 12 vs 16 intencional pineado `TestMCPIntentionalGapPinsPluginDelta` (4-gap evolve_propose/probe_contrast/probe_compose/skills_list, policy Devin, docs/devin.md) — verde por diseño, sin cambio; stamp verde `doctor 0.8.1 (matches package.json)` sin WARNING (fix L2 intacto, troubleshooting unstamped-by-design) — sin cambio; stale rojo → fijado mínimo `memory reindex 29 obs sha256 73b73c6a76956249e598bac58a4b8f3f5e750ed7b98f884df45b7cc8480aa779` → `memory doctor fresh (hybrid ranking active)` + `search` sin warning — verde; cap implícito no rojo (status 8/10, probe ok sin refused-cap; default 10 en budget.go:38 + troubleshooting cap-doc + reflect caps) — documentado, sin cambio a explícito; proposer estático por diseño (evolve.go:1 assembly read-only, autoría agente/guardian; non-goal vanguard S11 benchmarks/judges/USD/k-scaling/Codex) — no implementado; resolveDomain frágil por diseño defensivo (domain.ts:289 try/catch nunca rompe hook) — sin cambio; timeout largo no rojo (mutation 43341ms<600s, vulncheck 2685ms<120s, DefaultTimeoutSec 600 gates.go:72) — sin cambio. Non-goals no tocados: proposer autónomo ni USD ledger (v0.5-vanguard S11). `gate run live-plus-gaps/L3` 12/12 pass (typecheck 1073ms, lint 512ms, build 641ms, unit-tests 24620ms, coverage-floor 674ms, mutation 43341ms, crap 366ms, vulncheck 2685ms, spec 356ms, ts-typecheck 425ms, ts-unit-tests 1358ms, gofmt 879ms). Receipts en `.regulus/evidence/live-plus-gaps__L3/`: typecheck-1793377308179469700, lint-1793377309253046100, build-1793377309765734200, unit-tests-1793377310410968700, coverage-floor-1793377335036088400, mutation-1793377335711329200, crap-1793377379053182900, vulncheck-1793377379420200000, spec-1793377382106550600, ts-typecheck-1793377382463170500, ts-unit-tests-1793377382888263500, gofmt-1793377384246833000. Sin tocar L1/L2.
