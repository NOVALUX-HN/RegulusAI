# Ahorro de tokens por mecanismo — medición antes/después (T4)

Spec T4 (verbatim de `charts/macizo-tokens.md:41-45`, no parafraseado):

- Given F1 (contabilidad) + T1–T3 activos
- When se mide antes/después en este repo con coste propio incluido
- Then el reporte de ahorro es honesto por mecanismo, con reversión documentada si alguno no paga

## Proxy honesto (F1)

El host no expone tokens: `trace cost` global reporta **3410 llamada(s),
40 error(es) en 139 sesión(es), todas `(unattributed)`**; tiempo y bytes
`unknown (unmeasured)` en líneas legacy; tokens
`unknown (host exposes none)`. `trace cost --task <t>` devuelve
`no entries attributed` para toda task histórica. Todo ahorro abajo se
mide en **proxies auditables** (chars/líneas del resume, definiciones +
guidance chars, líneas de nudge), nunca en tokens estimados.

## Tabla por mecanismo (medido en este repo, 2026-11-01)

| Mec. | Antes | Después | Ahorro medido | Coste propio (incluido) | Reversión | Veredicto |
|------|-------|---------|---------------|-------------------------|-----------|-----------|
| T1 compaction acotada (defaults 20/20) | resume explícito-full `--limit 0 --budget 0`: **25767 chars, 226 líneas** | resume default acotado: **15364 chars, 187 líneas** | **−10403 chars (−40.4%), −39 líneas (−17.3%)** | líneas `showing latest N of M` ya dentro de la cifra bounded (ahorro **neto**); 1 lectura de env por invocación | `REGULUS_COMPACTION_FULL=1` restaura el resume completo | **PAGA** |
| T2 subsets por modo (load-time) | full: **16 defs, 1362 guidance chars** | minimal: **3 defs, 334 guidance chars** | **−13 defs (−81.25%), −1028 guidance chars (−75.5%)**; minimal = 24.5% del full (< 1/2, pin `toolset.test.ts:94`) | `resolveToolset` 1 vez por load (env + scan `charts/`); header compartido 73 chars contado en ambos lados (ahorro neto); fail-closed full | `TE_TOOLSET=full` restaura las 16 | **PAGA en modo minimal; 0 en este repo** (con `charts/` el auto resuelve full por diseño) |
| T3 loop guard (×3 → redirect 1 línea) | **SINTÉTICO** (etiquetado, ver abajo): 10 fallos idénticos sin guard → 0 señales | mismo escenario con guard → **3 nudges** (en #3, #6, #9), **1 línea / 209 chars c/u** | bucles evitados en real: **NO MEDIBLE** (0 incidentes registrados; el guard es advisory, nunca detiene) | `normalize` ≤200 chars + 1 entrada/sesión (cap 200) por fallo; 209 chars por cada racha de 3 | `TE_LOOPGUARD=off` (también `0/false/no/disabled`) desactiva el nudge | **beneficio NO MEDIBLE; coste despreciable; se mantiene por opcionalidad, no por ahorro probado** |

Notas por mecanismo:

- **T1.** Medido sobre el store real del repo (`memory context` vs
  `memory context --limit 0 --budget 0`, misma tubería → el ratio es
  independiente de la codificación). Pin histórico al cierre de T1
  (`f7f3a2a`, store de entonces): −58% (−9227 chars, 70 vs 100
  líneas); la cifra fresca (−40.4%) refleja el store crecido desde
  entonces, no una regresión. Fidelidad intacta:
  `TestBoundedResumeKeepsEssentials`
  (`internal/almanac/context_fidelity_test.go`): pins, últimas
  decisiones y lessons sobreviven al cap.
- **T2.** Medido con el mecanismo real (`parseToolGuidance` +
  `toolsetTools` sobre `.opencode/tool-guidance.md`). En este repo el
  ahorro efectivo es 0 porque `auto` resuelve `full` con `charts/`
  presente — es el diseño documentado, no una fuga: el ahorro aplica
  a sesiones simples sin charts. Ningún mecanismo paga aquí y a la
  vez en todas partes; se declara dónde paga cada uno.
- **T3 (SINTÉTICO — no es baseline real).** No hay incidentes reales
  de bucle en el repo: el trace histórico está sin atribuir por task
  (F1) y el guard nunca detiene por sí solo. El escenario (10 fallos
  idénticos de `gate_run` contra el `LoopGuard` real de
  `.opencode/lib/domain.ts`) demuestra disparo determinista y coste,
  no ahorro. Contarlo como ahorro sería inventar baseline.

## Coste de la medición (incluido, no externo)

- 2 corridas `memory context` (full + bounded): **~261–266 ms wall
  c/u** (incluye overhead de `go run`; el binario es más rápido).
- 2 scripts bun efímeros (guidance chars, escenario sintético):
  eliminados tras medir; < 1 s máquina en total.
- Este doc: ~4 KB que el lector paga una vez; no entra al contexto
  de sesión (no lo inyecta ningún hook).

## Reversión (toda reversible, toda env-gated)

- T1: `REGULUS_COMPACTION_FULL=1` → resume explícito-full.
- T2: `TE_TOOLSET=full` (o typo/desconocido → fail-closed full).
- T3: `TE_LOOPGUARD=off` → sin nudge.
- Ninguna reversión cambia conducta de gates; las tres están
  documentadas donde actúan (hook, setup, línea de nudge).

## Límites (honestos)

- Proxy, no tokens: el host no expone conteos (F1 cero honesto).
- n pequeño: 1 store (T1), 1 guidance-md (T2), 1 escenario sintético
  n=10 (T3). Un solo runner (`isolation: host-contained` en recibos).
- T2 sin efecto medible en este repo (resuelve full por diseño).
- T3 sin datos reales: beneficio declarado no medible, escenario
  sintético etiquetado como tal.
- Ningún mecanismo revierte en esta tarea: T1 paga, T2 paga donde
  aplica, T3 cuesta ~0. Si alguno dejara de pagar, su reversión está
  en la tabla de arriba.
