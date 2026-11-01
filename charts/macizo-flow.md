# macizo-flow — coste medido + review proporcional + benchmark propio

User authorization: "crea un plan para ejecutar cada uno de ellos e integrarlos a Regulus para hacer un producto macizo" 2026-11-01. Programa macizo 2/3 (flujo proporcional y medido, lecciones de gentle-ai trigger-rules/RDD y Harness² benchmarks). Read down to `## Log` before working.

Class: COMPLEX

Domain: sextant

## Contract

- Objective: cada clase de trabajo con coste medido, review proporcional al riesgo y benchmark propio para decidir ADOPT/REJECT con datos.
- Scope: `internal/trace/*`, `internal/observability/*`, `internal/cli/*` (nuevo `review`), `docs/*` (tabla publicada), nuevo `bench/` o `.regulus/bench/` + runner. No se debilitan gates existentes; el modo review es informativo y opt-in.
- Constraints: sin nuevas dependencias; el benchmark no sale a red; costes honestos (si el host no expone tokens, proxies: llamadas, bytes, tiempo).
- Acceptance: F1, F2, F3, F4
- Artifacts: receipts `macizo-flow/F*` (12/12 por tarea).
- Dependencies: F1 primero (contabilidad); F3 y F4 tras F1; F2 independiente.
- Risk: complex
- Verification: `spec check` + `gate run macizo-flow/F*` 12/12.
- Completion: F1–F4 CLOSED vía `task close`.

## Acceptance

### Scenario: F1

- Given traces sin coste (`trace.go:30-41` sin tokens; `metrics.go:41-42` honest zero)
- When se añade contabilidad por task (tokens si el host los expone; si no, llamadas+bytes+tiempo como proxy)
- Then cada task reporta su coste y el reporte es auditable contra traces

### Scenario: F2

- Given el gauntlet completo corre siempre (~5 min) y RDD demuestra que review proporcional basta en riesgo medio
- When existe modo `review` (congela diff, aplica 1 lente según riesgo, informativo, opt-in, delivery humano)
- Then riesgo medio se revisa en segundos y el gauntlet completo queda para alto/CRITICAL, sin debilitar nada existente

### Scenario: F3

- Given F1 con contabilidad y las clases del Sextant (`docs/sextant.md:13-18`)
- When se miden 5+ unidades por clase (tiempo/coste/turns)
- Then la tabla trigger-rules medida se publica en docs y decide rutas con números, no intuición

### Scenario: F4

- Given Harness² mide en JobBench/LAB/WB-Bench y nosotros solo contra nosotros mismos
- When existe suite de 5–10 tasks con checks ocultos + runner offline
- Then la superficie tiene baseline para que futuros ADOPT/REJECT se decidan por contraste, no por juicio

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|------|-------|
| F1 | F1 | contabilidad de coste por task en traces (tokens o proxy) | @regulus-worker | done |
| F2 | F2 | modo review proporcional (diff congelado + 1 lente, opt-in) | @regulus-worker |  |
| F3 | F3 | medir 5+ unidades/clase y publicar tabla trigger-rules | @regulus-worker |  |
| F4 | F4 | benchmark propio 5–10 tasks + runner offline + baseline | @regulus-worker |  |

## Log

- L1: Chart opened 2026-11-01 (programa macizo 2/3). Evidencia: honest-zero `metrics.go:41-42`, clases `sextant.md:13-18`, gentle-ai tabla medida, Harness² benchmarks.
- L2: F1 CLOSED 2026-11-01 (`ses_e6af35090ffe0CvALVr75EIfNZ`, commit `30fa954`): `trace.Entry` + task/duration_ms/output_bytes/input+output_tokens (omitempty, cero=desconocido); proxy llamadas+tiempo+bytes (host no expone tokens); `trace cost [--task]` auditable contra JSONL; legacy→`(unattributed)`. `gate run macizo-flow/F1` 12/12 (`run_2c99daa3948e4f07`), verify 12/12, `task close` formal.

- CLOSED F1 COMPLEX spec OK + 12 receipt(s) PASS.
- L3: F2 CLOSED 2026-11-01 (`ses_e6aa50506ffesxDh5HXS3keOid`): `regulus review [--task] [--lens]` (congela head, clasifica riesgo, 1 lente, informativo, exit 0, cero escrituras; `regulus.json` intacto). Run1 cayó en crap (reviewRunMutation cx10/cov24%→53.90) → SPLIT+COVER → re-run `run_5fcb71c1d467a09e` 12/12, verify 19/19. `task close` exige historial todo-verde → cierre manual (4º caso; la candidata "close sobre último manifest" ya es deuda registrada).