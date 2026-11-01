# Trigger-rules medidas — rutas con números, no intuición (F3)

Spec F3 (verbatim de `charts/macizo-flow.md:35-39`, no parafraseado):

- Given F1 con contabilidad y las clases del Sextant (`docs/sextant.md:13-18`)
- When se miden 5+ unidades por clase (tiempo/coste/turns)
- Then la tabla trigger-rules medida se publica en docs y decide rutas con números, no intuición

Clases (`docs/sextant.md:13-18`): TRIVIAL (inline, nunca delega),
STANDARD (una unidad de código), COMPLEX (varias unidades o
dependencias), CRITICAL (destructivo/seguridad/contratos/concurrencia/
delivery/riesgo incierto + verifier).

## Metodología

- **Unidad medida** = una corrida completa (12/12 recibos PASS) del
  gauntlet para una task, identificada por `run_id` de su manifest
  (`evidence ... list` / `manifest-run_*.json`). Solo corridas con
  manifest: los recibos legacy sin `run_id` agregan varias corridas y no
  son atribuibles a una sola unidad.
- **Tiempo** = Σ `duration_ms` de los 12 recibos (wall del gauntlet).
- **Coste proxy** = el mismo tiempo Σ `duration_ms`. El host no expone
  tokens (F1: honesto cero) y `trace cost` global reporta 3184
  llamada(s) en 136 sesión(es) **todas `(unattributed)`**:
  `trace cost --task <t>` devuelve `no entries attributed` para toda
  task histórica. Llamadas/bytes por unidad: no medibles en el
  histórico.
- **Turns** = n/d: los recibos no llevan contador de turnos y las
  sesiones de trace no están atribuidas a tasks. No se inventa.
- **Clase de cada unidad** = `Class:` del chart de su task.
  Corridas de charts sin `Class:` (legacy) quedan fuera de la tabla
  por clase.
- Corpus: 60 manifests → 66 corridas con `run_id` → **48 completas
  (12/12)** medibles; 18 parciales (n∈{1,3,4,7,11}, gauntlets
  interrumpidos/fallidos) excluidas y contadas en Límites.

## Tabla por clase (tiempos wall Σ duration_ms)

| Clase    | n (12/12) | Tiempo mediano | Media  | Min – Max      | Coste proxy mediano¹ | Turns |
|----------|-----------|----------------|--------|----------------|----------------------|-------|
| TRIVIAL  | 0         | n/d            | n/d    | —              | n/d                  | n/d   |
| STANDARD | 0         | n/d            | n/d    | —              | n/d                  | n/d   |
| COMPLEX  | 38        | **187.8 s** (~3.1 min) | 216.4 s | 128.1 – 438.5 s | 187.8 s wall (tokens unknown) | n/d |
| CRITICAL | 10        | **165.8 s** (~2.8 min) | 172.2 s | 133.9 – 271.5 s | 165.8 s wall (tokens unknown) | n/d |

¹ Proxy F1: host sin tokens; llamadas/bytes históricos sin atribuir.
El proxy por unidad es tiempo hasta que el trace atribuya por task.

TRIVIAL y STANDARD tienen **n=0 honesto**: ningún chart del repo
declara esas clases (todo el histórico ejecutado es COMPLEX o
CRITICAL), así que no hay 5+ unidades medibles. Sus rutas se mantienen
**por construcción, no por medición** (marcado como intuición
pendiente en Decisiones, con criterio de salida).

Muestra fresca (n=1, fuera del corpus histórico):
`review --task macizo-flow/F3` = **705 ms wall**
(incluye overhead de `go run`; el binario es más rápido)
frente a gauntlet mediano ~166–188 s → el review cuesta **~1/250**
del gauntlet.

## Tabla 12 capas (medianas del corpus, n=48 corridas completas)

Corrida de referencia (todas PASS): `run_5fcb71c1d467a09e`
(`macizo-flow/F2`, total 190.0 s, verificable con `evidence show`).

| Capa | Gate           | Mediana corpus | Media corpus | Ref. run (ms) | Recibo ref.            | Peso² |
|------|----------------|----------------|--------------|---------------|------------------------|-------|
| 1    | typecheck      | 1226 ms | 1273 ms | 1242 | rec_18e3d1c17c0b0214 | 0.7% |
| 2    | lint           | 789 ms  | 810 ms  | 854  | rec_18e3d1c1c694ea70 | 0.4% |
| 3    | build          | 874 ms  | 878 ms  | 1123 | rec_18e3d1c1f9e8d65c | 0.5% |
| 4    | unit-tests     | 139140 ms | 164757 ms | 151497 | rec_18e3d1c23d3f8c84 | 78.4% |
| 5    | coverage-floor | 740 ms  | 748 ms  | 746  | rec_18e3d1e58397b620 | 0.4% |
| 6    | mutation       | 27784 ms | 30922 ms | 27644 | rec_18e3d1e5b07ca8a8 | 15.7% |
| 7    | crap           | 452 ms  | 1204 ms | 455  | rec_18e3d1ec23c6b028 | 0.3% |
| 8    | vulncheck      | 3108 ms | 3133 ms | 3120 | rec_18e3d1ec3f5fe070 | 1.8% |
| 9    | spec           | 428 ms  | 498 ms  | 433  | rec_18e3d1ecfa1d36d8 | 0.2% |
| 10   | ts-typecheck   | 472 ms  | 474 ms  | 458  | rec_18e3d1ed1464f404 | 0.3% |
| 11   | ts-unit-tests  | 1402 ms | 1473 ms | 1440 | rec_18e3d1ed30242728 | 0.8% |
| 12   | gofmt          | 957 ms  | 1008 ms | 979  | rec_18e3d1ed8664e320 | 0.5% |

² Peso = mediana capa / Σ medianas (177.4 s). Capas 4+6 = **94.1%**
del wall. Las otras 10 capas juntas ≈ 10 s (5.9%).

## Decisiones de ruta con números

1. **Riesgo medio → review primero (F2 confirmado)**: review ~0.7 s
   vs gauntlet mediano 166–188 s. El review cuesta ~1/250 del
   gauntlet; prescribe review-primero para STANDARD/COMPLEX no
   críticos y reserva el gauntlet completo para alto/CRITICAL.
2. **CRITICAL no cuesta más máquina que COMPLEX** (med 165.8 s vs
   187.8 s; medias 172.2 vs 216.4 s): el rigor cuesta humano
   (verifier obligatorio), no tiempo de gates. Se mantiene el
   verifier sin objeción de coste máquina.
3. **La palanca no es saltar capas baratas**: las 10 capas fuera de
   4+6 suman ~10 s; cualquier ruta que las omita ahorra <6%.
   La optimización real (alcance de tests/mutación) pertenece a
   trabajo futuro, no a recortar gates.
4. **TRIVIAL→inline y STANDARD→Chart+gauntlet: sin datos (n=0)** —
   se mantienen por construcción. Criterio de salida: cuando haya
   5+ unidades TRIVIAL/STANDARD ejecutadas, re-medir y contrastar
   contra esta tabla antes de tocar sus rutas.

## NOTAS (propuestas, sin cambiar rutas en esta tarea)

- **NOTA-F3-1**: no se encontró ruta por defecto desproporcionada
  en los defaults medibles. `task check charts/macizo-flow.md`
  responde `OK (class COMPLEX, 4 task(s))`: la regla
  "TRIVIAL con marcador de riesgo se rechaza, nunca delega"
  (`sextant.md:21`) sigue machine-checked; no hay evidencia de
  TRIVIAL que delegue (no existe ni una unidad TRIVIAL).
- **NOTA-F3-2**: la única desproporción medida está *dentro* del
  gauntlet, no en las rutas (capas 4+6 = 94.1%). No se propone
  recortar nada aquí; se deja como insumo para F4 (benchmark
  propio) y futura optimización de alcance de tests.

## Límites (honestos)

- n pequeño y desigual: CRITICAL n=10 frente a COMPLEX n=38;
  TRIVIAL/STANDARD n=0 (sin 5+ por clase en esas dos).
- Un solo runner (`isolation: host-contained` en recibos): sin
  contraste entre máquinas; wall-clock, no CPU normalizado
  (`cpu_time_ms` existe en recibos nuevos pero no se agrega aquí).
- Sin pricing de caché/tokens: el host no expone tokens, proxy =
  tiempo; llamadas/bytes históricos sin atribuir por task.
- 18 corridas parciales excluidas (gauntlets interrumpidos o con
  fail); mediana sobre 48 completas 12/12 PASS.
- Review: muestra fresca n=1 con overhead de `go run`.
- Veredicto de cobertura: **parcial por construcción del
  histórico** — la tabla decide rutas donde hay datos (COMPLEX,
  CRITICAL, review-vs-gauntlet) y declara intuición pendiente
  donde no los hay (TRIVIAL, STANDARD), con criterio de salida.
