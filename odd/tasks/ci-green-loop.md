# ci-green-loop — CI absolutamente verde

Header: loop autónomo hasta que Gauntlet + Windows estén verdes. Commits en
unidades revisables sobre `develop`, push y `gh run watch` hasta PASS.

## Specs

- S1: `regulus crap --profile .regulus/coverage.out --max 30 --max-critical 30 --max-standard 50 --top 5` con max < 30 (los 5 offenders de CI del run 43684619668 bajo 30).
- S2: `gh run watch` del push muestra Gauntlet PASS (16 layers, incluye crap).
- S3: Windows compatibility PASS: windows-native 0 FAIL y windows-protected-audit PASS.
- S4: sin cambios de comportamiento: mismos mensajes, mismos exit codes; `go build ./...` + `go vet` + tests de paquetes tocados en verde local.
- S5: loop commit→push→watch→(rojo? fix)→... hasta S2+S3.

## Tasks

| ID | Spec | Work | Estado |
|----|------|------|--------|
| T1 | S1 | coverage fresco local (paquetes tocados) + baseline crap | in_progress |
| T2 | S1,S4 | COVER `Report.TierMaxima` (cx6 cov0) | pending |
| T3 | S1,S4 | `benchHostJourneysCmd` (cx19 cov57.9): COVER a ~70%+ o split leve | pending |
| T4 | S1,S4 | SPLIT `checkToolManifest` (cx41) | pending |
| T5 | S1,S4 | SPLIT `RunExplorationResume` (cx46) | pending |
| T6 | S1,S4 | SPLIT `execAndReport` (cx31) | pending |
| T7 | S3,S4 | triage windows-native: clasificar 233 fallos, fix código vs entorno | pending |
| T8 | S2,S3,S5 | commit + push + watch; repetir hasta verde | pending |

## Log

- L1 (usuario, verbatim): "me ire a dormir. quiero que corrijas todos los errores que dice el CI, luego haz el commit y el push y vuelve a revisar que todo este OK con el CI, si vuelve a fallar vuelve a corregir los errores mostrados. crea ese bucle hasta que el CI pase absolutamente todo"
- L2: run 43684619668 (push 98d4e41): Gauntlet cae en layer 9 crap (49.07: checkToolManifest 49.07, RunExplorationResume 48.90, benchHostJourneysCmd 45.95, TierMaxima 42.00, execAndReport 36.90). Windows: 0 tests nuevos rotos vs run previo (234→233), todo preexistente/entorno.
- L3: crap pagado local (max 29.84): T2 COVER TierMaxima (test nuevo, 100%); T3 SPLIT benchHostJourneysCmd en 6 helpers + 5 tests; T4 SPLIT checkToolManifest en 5 helpers; T5 SPLIT RunExplorationResume con explorationRunner (caza carrera real en spy de f7_e2e, fix con mutex); T6 SPLIT execAndReport en 4 helpers (mensajes byte-identicos); +3 COVER (rollback/audit/evaluate) + insurance GovernedRollback (70.8%, CRAP ~24.2).
- L4: Windows triado — causa raiz: GOTMPDIR dentro del repo => t.TempDir() cae en el checkout => ResolveRoot devuelve repo (aislamiento roto, 174 cli + stores con 239 sesiones reales) + tests con `../..`/cwd-relativo + gremlins ausente en job. Fix: GOTMPDIR a sibling fuera del repo, runtime.Caller anchors (manifest x4, bench shape x2, gates regulus, adversarial catalog+corpus, manifest tools), gremlins@v0.6.0 en windows.yml. Verificado con el script: 34 PASS, 1 FAIL (bench agent-suite x2, solo entorno local sin node/bash en PATH de PowerShell; en CI pasan).
- L5: 2 commits listos (0a743bf crap + c2d42c9 windows). Push BLOQUEADO por RDD pre-delivery: assess reviewDue=true (native high por shell en gate_run.go + windows.yml); START declinado 2x sin linaje ni envelope; el gate sigue exigiendo consentimiento humano. Loop en pausa hasta decision humana.
- L6: paridad local↔CI: journey tenia 3 fallos mas (F2 repoRoot + S8 repoRoot x2, mismo patron Getwd+../.. — anclados a runtime.Caller en 9224330). F2: fixture mutation necesitaba --timeout-coefficient 10 como los gates del repo (en Windows los mutantes agotan el timeout por defecto); gremlins da 100% en el fixture. Full script local: 32 PASS + bench x2 (sin node/bash en PATH de PowerShell, en CI existen) + journey F2 (loteria App Control local; pasa en retry) + gates BLOCKED (loteria). Script impone STRICT_ENV=1 como el job; doc de paridad agregada.
- L7: single ordered workflow: ci.yml fusiona gauntlet+windows (mismos jobs, steps, pins, timeouts verbatim; job ids preservados) + pre-commit local documentado (gate run ci/push + test-windows.ps1). race-almanac: verdict perf con stand-down bajo -race (build tags), layer 5 sigue dueno del techo.
- L8: CI corre con los fixes (14/15: cae solo race-almanac perf bajo -race, 0 data races) -> fix con build tags; el usuario pushea y CI cae en unit-tests por TestS8GateEfficacy (leia gauntlet.yml eliminado) -> apunta a ci.yml; verificado local.
