# learn-backfill — lecciones de lo trabajado desde 0.11.0

User authorization: "otra cosa que es necesario de revisar es el hecho que el aprendizaje hasta ahora con todo lo que e trabajado con Regulus aun haya sido de 0. verifica si hay algun problema con eso" + "haz todas las tareas que detectes sin esperar mi autorizacion" 2026-11-02. Hallazgo: solo 2 lecciones admitidas (go, sextant, ~26 oct) y 0 candidatos; `reflect` no se corrió para ~40 tasks cerradas desde 0.11.0 (proceso omitido, no código roto). Read down to `## Log` before working.

Class: STANDARD

## Contract

- Objective: el Almanac vuelve a aprender: 2 lecciones admitidas con evidencia de trabajo real reciente (ids duplicados HTML; timeout infra exit −1 con todo ok).
- Scope: `regulus reflect` (2 candidatos) + validación de guardián + `regulus library admit`. Cero código.
- Constraints: cada lección cita receipts que existen y pasan; GENERALIZE (transferible); nada inventado; admisión solo con guardián que la valide.
- Acceptance: R1, R2
- Artifacts: candidatos + receipts de validación (notas de sesión).
- Dependencies: R1 y R2 independientes.
- Risk: standard
- Verifier: R1, R2 (el guardián valida antes de admitir).
- Verification: `regulus library list` muestra 4 lecciones (2 viejas + 2 nuevas) con receipts citadas.
- Completion: R1–R2 CLOSED vía `task close`.

## Acceptance

### Scenario: R1

- Given `dashboard-vision/V1` cazó `id="graphs"` duplicado preexistente que rompía el panel cada 30s (evidencia: MCPs + run_4e4eaa24b4f26b32 si verde)
- When se admite la lección "ids duplicados HTML: valida unicidad / verifícalo en vivo"
- Then `library list` la muestra con sus receipts y aplica a futuro (checklist dashboard)

### Scenario: R2

- Given `unit-tests` exit −1 con todos los paquetes `ok` en 4+ gates (infra timeout, re-run caliente verde: dashboard-ux/U2, dashboard-vision/V1, win-publisher/T1+T2)
- When se admite la lección "exit −1 + todo ok = timeout infra: re-lanza en caliente antes de tratarlo como fallo"
- Then `library list` la muestra con sus receipts

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| R1 | R1 | reflect ids-duplicados + admit con guardián | @regulus-worker | done |
| R2 | R2 | reflect timeout-infra + admit con guardián | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-02 (2 admitidas viejas, 0 candidatos, ~40 tasks sin reflect desde 0.11.0).
- CLOSED R2 STANDARD: candidato les_18e41eaf10e2e8cc admitido con regulus-evidence-auditor (fail -1 infra ×2 → pass caliente run_4e4eaa24b4f26b32, 34 receipts). R1 PENDIENTE: les_18e41eaff0481618 UNBACKED hasta dashboard-vision/V1 verde + receipt de navegador vivo.
- CLOSED R1 STANDARD: les_18e41eaff0481618 admitido con regulus-evidence-auditor tras artefacto live-graphs-1793593050000000000.json (12 recs V1 verdes + 0 errores en 3 ciclos, 13 receipts). Librería sextant: 4 lecciones.
