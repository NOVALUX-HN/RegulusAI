# gate-hardening — allowlist real, jail de paths, untar seguro, LCS cubierto

User authorization: plan total 2026-10-31 ("haz un plan y usa el todos... hasta resolverlos todos"), primero = P0 seguridad. Read down to `## Log` before working.

Class: CRITICAL

Domain: sextant

## Contract

- Objective: cerrar bypasses de ejecución y paths + cubrir el LCS de probe.
- Scope: `internal/gates/run_helpers.go` (allowlist), `internal/receipts/` + `internal/evidence/` (Slug/jail), `internal/gates/mutation_export.go` (untar), `internal/probe/compose.go` (LCS) + tests. Sin cambiar thresholds ni UX.
- Constraints: mismos mensajes de refuse (extiende, no reescribe); `Slug` actual preservado donde es contrato de paths (solo añade validación); tests journey verdes.
- Acceptance: S1, S2, S3, S4
- Artifacts: receipts `gate-hardening/G*`.
- Dependencies: G1–G4 independientes; V1 al final.
- Risk: CRITICAL (seguridad: bypass ejecución, traversal, exec-bits) — requiere verifier.
- Verification: `spec check` + `gate run gate-hardening/G*` + V1 intenta los bypasses y confirma bloqueo.
- Completion: 4 tasks CLOSED + V1 PASS, `spec OK: 4`.

## Acceptance

### Scenario: S1 allowlist multi-segmento

- Given gate `echo hi; rm -rf /` o `go test || chmod -R /`
- When corre el refuse-policy check
- Then rehúsa (cada segmento contra allowlist), con el mismo mensaje `policy:` existente

### Scenario: S2 paths con jail

- Given `Slug("../evil")` o task con `..`/absoluto
- When `TaskDir`/`Plan`/`Verify` resuelven
- Then rehúsan sin salir de `.regulus/evidence/` (defensa en profundidad aunque el caller valide)

### Scenario: S3 untar sin exec-bits

- Given tar con entries `0755/0777`
- When el export hermético extrae
- Then ficheros quedan sin exec-bits (máscara `0o644` base) y gremlins corre igual

### Scenario: S4 LCS cubierto

- Given helpers `mergeSegments`/`walkClusters`/`lcsTable`/`lcsWalk`/`replaceTail`/`extendRun` without cover
- When corren casos inserción-abutting-cambio + bordes
- Then veredictos exactos y `go test ./internal/probe/` verde

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| G1 | S1 | allowlist por segmentos (`splitSegments` en refuse-policy) + tests bypass | @regulus-worker |
| G2 | S2 | jail en `TaskDir`/`Plan`/`Verify` (rechaza `..`/absolutos/escape) + tests | @regulus-worker |
| G3 | S3 | untar sin exec-bits + tests | @regulus-worker |
| G4 | S4 | tests LCS (abutting, bordes, off-by-one) | @regulus-worker |
| V1 | S1-S4 | verificación independiente: reproduce bypasses `;`/`||`/`$VAR`, traversal `..`, tar 0777 y confirma bloqueo + gate capa 7 pass | @regulus-verifier |

## Log

- L1: Chart opened 2026-10-31 (plan total, P0). Scout Go `ses_e6ba201acffeCkBZAhydahK6gi` §4–§6: allowlist primer-token (`run_helpers.go:73`) vs `Classify` full-scan (`:110`); `Slug` (`receipts.go:86`) sin jail en `TaskDir`/`Plan:48`/`Verify:90`; `Chmod` tar (`mutation_export.go:179`); LCS (`compose.go:414-492`).
- L2: G1–G4 CLOSED + V1 PASS 2026-10-31 — G1 allowlist segmentos (`ses_e6b9e4258ffeR10CXTACVd5x2f`, 12/12), G2 jail (`ses_e6b93b61dffeFKPr4f5ARZ0rYH`, 12/12), G3 sin exec-bits (`ses_e6b8d742fffeGmkG2ldvqDZM0X`, 12/12), G4 LCS solo-tests (`ses_e6b898090ffe59eTlsssXgDSxF`, algoritmo correcto, 12/12). V1 (`ses_e6b862480ffe1MWt3KuJ0uhCj5`) CONFIRMED S1–S4 + gate propio 12/12 (`run_0047758822e6be30`, crap 29.46). `spec OK: 4`. Cierre manual.
