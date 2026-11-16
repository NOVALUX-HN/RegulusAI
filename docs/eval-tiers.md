# Evaluación escalonada L0–L3 (F11.3 / NOV-89)

Objetivo: reducir el tiempo de feedback sin eliminar tareas, bajar
dificultad ni contaminar el holdout. El corpus (`agent-tasks-v3`,
44 tareas, 10 holdout) **nunca se recorta**: los niveles seleccionan
un subconjunto para correr y explican cada inclusión y cada omisión.

## Niveles

| Nivel | Nombre | Qué corre | Holdout | Techo run |
|-------|--------|-----------|---------|-----------|
| L0 | smoke | 4 tareas fijas de exploración, una por lenguaje (AT1 go, AT14 node, AT19 shell, AT23 python) | sellado | 300 s |
| L1 | focused | exploración cuya capacidad toca el diff (`--changed`) o el filtro (`--capability`) | sellado | 900 s |
| L2 | regression | muestra estratificada de exploración: cada lenguaje representado + trampas AT28/AT29 siempre | sellado | 1800 s |
| L3 | full+holdout | corpus completo (exploración + holdout), nada omitido | **incluido** | 7200 s |

Capacidades: `go | node | shell | python | gaming-trap | concurrency`.
Un cambio solo-docs (`*.md`, `docs/`, `charts/`, `odd/`) recomienda L0;
rutas harness (`internal/bench/`, `internal/gates/`, `internal/trace/`,
`internal/observability/`, `bench/`, `regulus.json`) ensanchan a todas
las capacidades. Los cambios pequeños **nunca** disparan L3 por
defecto: L3 solo llega por petición explícita o por política.

## Comandos

```bash
# Niveles y presupuestos
bin/regulus bench tiers [--json]

# Selección (auto recomienda desde el diff; explica incluidas/omitidas)
bin/regulus bench tier-plan [--tier auto|L0|L1|L2|L3] [--changed f1,f2] \
  [--capability C] [--agent A] [--release] [--last-l3 RFC3339] \
  [--jobs N] [--json] [--save P] [--evidence-dir D]

# Reporte verificado (falla si el plan fue editado: sha256)
bin/regulus bench tier-report --evidence-dir <dir>
```

Ejemplos:

```bash
# Smoke de un cambio de docs (auto -> L0, 4 tareas)
bin/regulus bench tier-plan --tier auto --changed docs/eval-tiers.md

# Foco Go de un cambio multi-archivo (auto -> L1)
bin/regulus bench tier-plan --changed internal/guardians/a.go,internal/guardians/b.go

# Regresión representativa
bin/regulus bench tier-plan --tier L2 --evidence-dir /tmp/tier-l2

# Full + holdout pre-release (escala aunque se pida L0)
bin/regulus bench tier-plan --tier L0 --release
bin/regulus bench tier-report --evidence-dir /tmp/tier-l2
```

El plan imprime el **run hint** exacto para ejecutar la selección por
el runner existente (mismos fixtures, splits y checks ocultos, sin
atajos):

```bash
regulus bench agent-tasks --task AT1,AT14,AT19,AT23 --split exploration
```

## Selección: inclusión y omisión

Cada plan registra nivel, diff, filtro, fingerprint del corpus
(`corpus_sha256`), seed de splits y, por tarea omitida, su razón:

- L0: `L0 smoke runs one <lang> task (<id>); <id> omitted (smoke set fixed)`.
- L1: `L1 focused on "<cap>": <id> carries <caps>, not selected` (o
  `untouched by the diff` en modo auto).
- L2: `L2 representative sample: <id> omitted by stratification,
  <lang> coverage kept by the sampled sibling`.
- Holdout bajo L3: `holdout sealed below L3: runs only at L3
  (periodic or pre-release)`.

## Preservación (nada se redefine aquí)

Manifiestos, fixtures fijados por commit, checks ocultos, splits y
anti-leakage siguen viviendo en `internal/bench` y
`bench/agent-splits-v3.json`. La selección las **reutiliza**:

- Los ids de splits desconocidos fallan cerrado (`LoadAgentSplits`).
- Todo nivel bajo L3 con un holdout incluido falla (`holdout sealed`).
- Las trampas AT28/AT29 nunca exponen `zz_hidden*` en `Files`; sus
  razones de omisión no nombran el check.
- El plan fija `suite_version`, `splits_version`, `splits_seed` y
  `corpus_sha256`: cualquier edición de fixtures mueve el pin.

## Medición por fase

Cada intento tiered divide su pared en seis fases que siempre
reconcilian al total: `prep / queue / inference / tools / eval /
teardown` (`TierPhases`; `Total()` suma las seis, `SumTierPhases`
agrega intentos). El plan reporta techos; el run mide cada fase por
separado para atribuir cuellos de botella (F11.4 consume estas fases).

## Presupuestos y clasificación

- Por tarea: el pin del fixture (`BudgetS`, nunca elevado aquí).
- Por agente: 2× la tarea más lenta del corpus.
- Por run: techo del nivel (300/900/1800/7200 s).

Clasificación cerrada (`ClassifyTierStatus`): solo `pass`/`fail` son
evaluables; `timeout`, `infra` y `non-evaluable` nunca pasan; lo
desconocido degrada a `non-evaluable`, jamás a `pass`.

## Paralelismo

Solo tareas independientes (cada una en su workspace aislado),
concurrencia limitada (defecto 4, máximo 8, arranques secuenciales
para respetar rate limits). `PlanTierBatches` ordena determinísticamente
y `ValidateTierParallel` rechaza duplicados (compartirían fixture) y
concurrencias fuera de 1..8.

## Política L3 y holdout sellado

- `--release` escala a L3 siempre (pre-release exige full + holdout).
- `--last-l3 <RFC3339>`: un L3 de hace más de 7 días (o ilegible)
  escala a L3; un L3 fresco no.
- Bajo L3 el holdout está sellado y verificado (`VerifyTierHoldoutSeal`).

## Latencia de feedback (techos, no predicciones)

`EstimateTierCost` compara techos honestos: agregado = suma de
`BudgetS`, pared = agregado / paralelismo. Un nivel menor es más
rápido porque corre menos techos, nunca porque recorte calidad:

| Nivel | Tareas | Agregado (techos) | Pared ~4× |
|-------|--------|-------------------|-----------|
| L0 | 4 | ~6 min | ~2 min |
| L2 | 20 | ~31 min | ~8 min |
| L3 | 44 | ~71 min | ~18 min |

(Valores indicativos desde `BudgetS`; el run mide la pared real y las
fases por separado. El agregado se reporta junto a la pared: la pared
sola no normaliza.)

## Evidencia

`--evidence-dir` guarda `tier-plan.json + tier-manifest.json` con
`plan_sha256`: `tier-report` verifica el hash antes de renderizar
(manipulado = fallo, nunca un reporte bonito de una mentira).
Manifiestos, hashes y la comparación pared/agregado viajan en cada
plan; el verde lo confirma CI.
