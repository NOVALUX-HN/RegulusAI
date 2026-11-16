# Trazabilidad de cierres (NOV-64 / T45)

## Línea base medida (F5–F7, develop)

24 issues cerrados en 4 commits multi-issue, un único autor de git y
cierres en ráfagas de segundos en Linear:

| Commit | Issues | Alcance declarado |
|--------|--------|-------------------|
| `2fe01cf` | NOV-43, NOV-45, NOV-46, NOV-44, NOV-47 (F5, 5) | Runner Windows + CI + docs |
| `001c7ac` | NOV-48…NOV-55 (F6, 8) | Contrato + 3 adaptadores + registry + ODD/RDD + CodeGraph + conformidad |
| `3de37af` | NOV-56, NOV-57, NOV-58 (F7, 3) | Protocolo + manifest + motor |
| `0169c23` | NOV-59, NOV-60, NOV-61 (F7, 3) | Evaluador + promoción + E2E |

Lección: "Done" quedó respaldado solo por el comentario de quien
implementó. Desde F8, el cierre exige evidencia enlazada.

## Política

- Un issue implica al menos un commit que lo cite (`NOV-xx` en subject o cuerpo).
- Commits multi-issue solo justificados: trailer `Justified: <motivo>` en el mensaje o flag `--justified` al cerrar. Sin justificación, el cierre falla nombrando el commit.
- Formato de cierre en Linear: `## T<n> NOV-xx — Implementado (<qué>)` + `**Qué se hizo:**` + `**Resultado:**` (comandos, exit codes, commits, receipts).
- Verificación del harness al cerrar (`task close --issue`): commits existentes, diff dentro del Scope del Chart (NOV-32), receipts referenciados existentes y verificados (flujo `CheckClose` existente).
- P0: veredicto independiente registrado como receipt (evaluación de un guardian distinto del implementador, o humano). Reutiliza `guardian-independence` (T3-02) y `verdict-authority` (T1-01): el revisor igual al implementador se rechaza; sin `--implementer` no se puede juzgar independencia.
- Con `--issue`, la verificación corre **por defecto**. `--skip-traceability` la omite solo con `--justified <motivo>` explícito, queda impreso en stdout y CI nunca lo usa (nunca silencioso).
- `task close --verify-only`: corre todos los chequeos sin escribir nada (sin reescribir el chart, sin receipts, sin nota de memoria); exit 1 nombra cada hueco. Es la vía que usa CI por cierre, porque el chart no se puede inferir del diff de forma fail-safe.
- CI (`traceability`, job requerido en `ci.yml`): por cada `NOV-xx` citado en el rango del PR/push verifica commit citado y trailer `Justified:` en multi-issue; además corre la matriz de fixtures `TestTrace*`. Falla cerrado si el rango no se puede determinar.
- Para trabajo de agentes la independencia es otro modelo, otro runtime o un humano; el veredicto queda como receipt y la auto-revisión se rechaza (ya lo hace `checkIndependentReview`).
- Protección de ramas (paso manual de admin GitHub, no versionable en repo): PR obligatorio a `main`/`develop`, al menos una revisión de una cuenta distinta del autor, checks requeridos (`Gauntlet`, `Windows compatibility`, `Traceable close`, `Windows protected audit`), prohibición de push directo. `CODEOWNERS` rutea `internal/gates`, `internal/evolve`, `internal/hostcfg` y `.github/workflows` a revisión obligada.

## Línea base post-F8 (NOV-78, medida 2026-11-16)

`git log --grep` por issue F8: 10/10 issues con al menos un commit que los cita (38 citas totales), un único autor (`SEXTUS`), 0 veredictos independientes P0 observables. De acá en adelante el job `traceability` impide repetir el patrón de un solo autor sin revisión.

## Excepciones legítimas

Hotfix (post-fix con issue creado justo después), merge (agrupa commits ya enlazados), solo-docs (sin receipts: el gate docs-drift es la evidencia). Ninguna abre atajos silenciosos: quedan registradas con su motivo.

## Definición de Done (NOV-74 Paso 56)

"Done" = cada criterio de aceptación del issue enlaza el comando o
artefacto que lo demuestra (salida, exit code, receipt, JSON versionado
con hash). Un criterio cumplido parcialmente se declara como tal en el
comentario de cierre (`parcial: ...`), nunca se da por cumplido en
silencio. Estado intermedio propuesto en Linear (pendiente de un admin
de Linear para crearlo): `Implementado, sin medir` — código entregado
y verificado estructuralmente, evidencia de medición pendiente.

## Reconciliación F8 (NOV-74 Paso 56, 2026-11-16)

Fuente de criterios: reconstrucción desde `odd/tasks/regulus-f8-evidencia.md`,
`CHANGELOG.md` y docs del repo (texto verbatim de Linear no confirmado
en esta pasada; si un criterio difiere, se corrige la fila y se reabre).
Ningún criterio queda sin dueño.

| Issue | Criterio (fuente repo) | Estado | Evidencia | Tratamiento |
|-------|------------------------|--------|-----------|-------------|
| NOV-62 Paso 42 | host-usage neutral; flags trace; no mezclar estimado/real | parcial | F8: todo `unknown`; post-F8 `docs/host-usage.md` (opencode `measured-host` 2026-11-15, resto `unknown`) | mantener cerrado → dueño NOV-73 (Done parcial 1 host); resto multi-host sin dueño F9 → proponer seguimiento |
| NOV-69 Paso 45 | niveles declarado→contrato→host-probado; `bench host-journeys`; workflow | no cumplido en F8 (4 `no-ejecutado`) | `docs/conformity-matrix.md` Estado 2026-11-16: 3/3 `host-probado` + `bench/evidence/host-journeys-20261116/` (sha256) | mantener cerrado → dueño NOV-76 (devin `not-run` con razón+fecha, no requiere más) |
| NOV-68 Paso 43 | suite agent-tasks + splits ≥20% holdout; runner headless; buckets; anti-leakage | parcial (v1: 5 tareas, base 5/5, 1 rep) | `docs/agent-evals.md` (v3 44 tareas, 44/44 fixtures fallan pre-fix); `bench/agent-splits-v3.json`; 84+30 outcomes (exploración 51 pass/30 timeout/3 infra; holdout 25/4/1) | mantener cerrado → dueño NOV-77 |
| NOV-71 Paso 50 | baseline vs candidato `agent-compare`; control negativo; decisión humana | parcial (`TIE_COST_TIEBREAK`, ahorro no medido, humano adopta) | `docs/f7-cycle-real.md` | mantener cerrado → dueño NOV-80 |
| NOV-63 Paso 44 | versión; gates; tools; runtimes; comandos | cumplido aparente | `docs/docs-drift.md`, gauntlet intacto | mantener cerrado, sin dueño |
| NOV-64 Paso 46 | commit cita `NOV-xx`; multi-issue justificado; formato cierre; P0 veredicto | cumplido aparente | este documento (línea base F5-F7 + política) | mantener cerrado, sin dueño |
| NOV-65 Paso 47 | 8 fuzz targets; perf-baselines 7 benchs; `STRICT_ENV` | parcial (2 fail-closed OK; P95 fuera por entorno) | `docs/perf-baselines.md`, `fuzz.yml` | mantener cerrado, sin dueño (nota de entorno registrada) |
| NOV-66 Paso 48 | `test-windows.ps1` paquete×paquete; paridad local↔CI; exclusiones | cumplido aparente | `docs/windows-full-suite.md` | mantener cerrado, sin dueño |
| NOV-67 Paso 49 | split domain.ts; `size check`; excepciones | cumplido aparente | gate size 16, embed byte-idéntico | mantener cerrado, sin dueño |
| NOV-70 Paso 51 | matriz 6 targets; checksums+SLSA; `verify-published`; launcher fail-closed | cumplido aparente salvo publicación | `docs/installation.md`; tag push de prueba pendiente = decisión de publicación del operador | mantener cerrado; el tag push sigue siendo decisión humana aparte |
