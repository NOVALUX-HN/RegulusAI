# Impact Analysis y grafo de dependencias (F12.8 / NOV-103)

Predecir componentes, contratos y pruebas afectados **antes** de cambiar
código: inventario cerrado, grafo incremental con aristas verificables,
mapeo diff → consumidores/rutas críticas/tests, riesgo explicable,
suite enfocada con precisión/recall medidos, ciclos/ocultas/breaking y
benchmark documentado vs CodeGraph (no dependencia).

## Reúso (nada paralelo, nada duplicado)

- **Inventario**: `context.Inventory` (F12.3) es el walk de código/
  símbolos/dependencias (bounds, skip-dirs, git head, memoria). Impact
  pliega sus fragmentos a paquetes/APIs/dependencias y solo camina lo
  que el corpus no trae (tests, workflows, configs, comandos).
- **Tiers**: `bench.TierL0..L3` (F11.3) nombran el nivel de la suite
  enfocada (L0 smoke, L1 foco, L2 regresión con críticos/ampliación;
  L3 nunca por foco: solo periódica/pre-release).
- **Riesgo**: la severidad es el input observado de F12.4 (tasa de
  alertas/fallos 0..1); aquí no se recomputa, solo escala la base.
- **Traza**: cada comando `impact` registra run.start + task.end +
  run.end en el exec log (F11.1), best-effort.
- **Críticos**: `CriticalPackages` espeja `size.CriticalPackages`
  (gates, evidence, receipts, manifest, verdict, contract); un test
  pinnea el espejo para que nunca diverjan.

## Inventario

`regulus impact inventory [--json]` — módulos, paquetes, APIs
(símbolos exportados), config (`regulus.json`, `opencode.json*`,
`.codex/config.toml`), workflows (`.github/workflows/*.yml`),
dependencias (`go.mod` + imports externos) y comandos
(`cliDispatch["…"]` en `internal/cli/*.go`, sin importar `cli`:
el grafo no depende del paquete que describe).

## Grafo y verificación de aristas

`regulus impact graph [--json]` — `pkg:internal/b -> pkg:internal/a
[go-import] internal/b/b.go:3 abc123def456 2026-01-02T…`. Cada arista
lleva **fuente + commit/fecha**; verificación:

| Fuente | Qué es | Cómo se reproduce |
|---|---|---|
| `go-import` | import interno en `.go` no-test (posición exacta vía `go/parser`) | `grep -n <import> <file>` + `git log -1 --format='%H %cI' -- <file>` |
| `test-import` | import en `_test.go` → paquete probado (+ bundle mismo-dir) | idem |
| `cli-route` | `cmd:X -> pkg:internal/cli` (entrada `cliDispatch`) y `pkg:internal/cli -> pkg:internal/cmdx` (forwarder con `cmdx.New`) | `grep -n 'cliDispatch\["X"\]' internal/cli/*.go` + git log del file |
| `workflow-ref` | mención `internal/<pkg>` o `regulus <cmd>` en workflow | `grep -n 'internal/' <wf>` + git log |
| `config-ref` | mención en config raíz | `grep -n 'internal/' <cfg>` + git log |

Sin git (o file untracked): commit `worktree` + mtime de la
evidencia — con fuente y fecha, nunca indocumentado. Incremental:
`BuildGraph(root, prev, lookup)` reutiliza aristas cuya huella
(mtime+tamaño) no se movió (`Reused=true`, commit/fecha viajan).

## Diff → consumidores, rutas críticas, tests

`regulus impact affect --changed <a,b> [--severity N] [--json]` —
`affect: 1 changed, 2 direct, 1 consumers, 1 critical, 2 tests`
más `critical-path: pkg:internal/gates -> pkg:internal/app`.
Consumidores = alcanzables inversos; tests = bundles en el conjunto
+ bundle del mismo dir de cada paquete afectado + bundles con arista
al conjunto. Rutas críticas = caminos acotados (prof. 6, máx. 10)
del cambio a nodos críticos.

## Riesgo explicable

`base = 5·directos + 3·consumidores + 2·tests (+40 si hay críticos,
+15 si hubo ampliación)`; `score = min(100, base·(1+severidad))`.
Nivel: `high` si score ≥ 60 **o hay críticos** (el kernel nunca se
degrada por inferencia), `medium` si ≥ 25, si no `low`. Cada término
se imprime (`- 2 consumidor(es) transitivo(s) (+6)`); la severidad
cita su procedencia F12.4.

## Suite enfocada + precisión/recall

`regulus impact suite --changed … [--full t,…] [--failed t,…]` —
`suite: 2 focused of 9 (tier L2)` + `omitted: … (N sin camino
verificado)` + `precision/recall/f1 (hit a/b, actual c)`.
`--failed` fija lo realmente roto (defecto: full); `P = hit/predecidos`,
`R = hit/actuales` (actual vacío + predicho vacío = 1/1; actual
vacío = fallback a full). **Fallback conservador**: entrada sin nodo
→ suite completa + `warning` en stderr; **los tests de paquetes
críticos nunca se omiten** (van siempre en el foco).

## Ciclos, ocultas, breaking

- `regulus impact cycles` — ciclos (DFS determinista: tratar como una
  unidad) y `hidden:` (aristas no-import: cli-route/workflow-ref/
  config-ref, lo que un scan de imports no ve).
- `regulus impact breaking --before <inventory.json>` — paquetes y
  APIs exportadas eliminadas desde un `inventory --json` previo.

## Benchmark vs CodeGraph (documentado, no dependencia)

`regulus impact benchmark` — tiempo de build propio vs scan ingenuo
+ decisión pinneada: **grafo propio adoptado** (aristas verificables,
sin daemon/DB); CodeGraph no integrado (coste/frescura/cobertura sin
valor neto, ver `docs/codegraph.md`). Revertir exige benchmark con
precisión/recall medidos.

## CLI

`regulus impact inventory|graph|affect|suite|cycles|breaking|benchmark`
— ver `regulus impact --help`. Códigos: 0 ok, 1 fallo/refusal, 2 uso.
Toda mutación de análisis es de mentira: el motor es puro (propone,
nunca toca producción; F12.4-style).
