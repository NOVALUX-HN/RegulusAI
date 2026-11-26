# quality-adjusted-performance — F21 · Quality-Adjusted Performance & Governed Evolution

Linear: NOV-133 (padre). Descompuesta en **NOV-154..157** el 2026-11-26.

La fase no tenía hijas. Escribir código sobre una fase sin tareas es inventar alcance,
así que primero se descompuso. Baseline: `develop`, nunca `main`.

## Por qué cuatro hijas y en este orden

El padre pide "demostrar con experimentos reproducibles que las optimizaciones mejoran
resultados, no sólo proxies". Eso es una cadena, no una lista: no se puede promocionar
sin medir, no se puede medir sin línea base, y no se puede promocionar de forma gobernada
sin consumir una métrica que ya exista.

| ID | Linear | Depende de | Qué cierra |
|----|--------|------------|------------|
| T1 | NOV-154 | — | Auditoría de brechas y **línea base controlada**: qué está medido en host real, qué es proxy, seeds, N mínimo, holdout representativo o su ausencia demostrada |
| T2 | NOV-155 | T1 | **Métrica de eficiencia ajustada por calidad**: coste/tokens/tiempo/llamadas por tarea exitosa, con varianza apropiada al N y procedencia explícita |
| T3 | NOV-156 | T1, T2 | **Experimentos reproducibles** baseline vs optimización individual vs combinación, con holdout y sin sobreajuste |
| T4 | NOV-157 | T2, T3 | **Promoción gobernada** reutilizando `internal/evolve`, con rollback probado, y dashboard/trazas sólo donde haya brecha |

## El enganche que ya está esperando

`internal/context/promotion.go` (`Evaluate`, F19.4) se escribió sin consumidor en
producción y así quedó declarado. **T2 es su consumidor natural**: hoy el A/B de contexto
emite un veredicto a partir de un delta proxy; rutearlo por `Evaluate` lo haría decir
`inconclusive`, que es lo correcto. Ese cambio flipea una expectativa existente, y por
eso pertenece a una tarea con su propia verificación y no a un commit de paso.

## Specs

S1 — Demostrar con **experimentos reproducibles** que las optimizaciones mejoran
resultados, no sólo proxies.

S2 — Auditar benchmark/evals/holdout/task journeys/host-real evidence/metric provenance/
Evolution antes de construir nada.

S3 — Comparar baseline vs optimización individual vs combinaciones: recovery, knowledge,
context/tool, REQP/testing.

S4 — Calidad: correctitud, completitud de tarea, requisitos, calidad y efectividad de
tests, seguridad, regresiones, mantenibilidad.

S5 — Eficiencia: tokens medidos/estimados, cache/reasoning cuando exista, coste, tiempo,
llamadas, reintentos, contexto y recursos.

S6 — Normalizar coste/tokens/tiempo/llamadas por **tarea exitosa**; eficiencia ajustada
por calidad y coste de fallo/recuperación.

S7 — Varianza, seeds, repeticiones, captura de versión/modelo/host/config/entorno,
holdout y confianza estadística **apropiada al tamaño de muestra**.

S8 — Declarar explícitamente medido-en-host vs estimado/proxy/desconocido; no afirmar
paridad de host ni tokens reales sin evidencia.

S9 — Umbrales de calidad como guardas; **ninguna promoción por ahorro de coste a secas**.

S10 — Evolution: proposal → candidate → benchmark → evaluación → decisión →
canary/rollback/monitoreo, reutilizando el flujo gobernado existente.

S11 — Dashboard y trazas: actualizar **sólo** donde una brecha esté demostrada.

S12 — Exclusiones verbatim: "No duplicate Agent Lab, dashboard, trace, Evolution or
benchmark engine; no overfit; no promote unaudited changes; no claim of quality
improvement from synthetic-only validation."

## Regla de honestidad que gobierna toda la fase

Un número sin procedencia no es evidencia. En esta fase se puede reportar un proxy, pero
tiene que decir `proxy`. Y si la muestra no alcanza, se **describe**; no se afirma. Esa
regla ya está escrita en el código (`internal/trace` para procedencia, `internal/bench`
para mínimos de N) y la fase la hereda en vez de reinventarla.

## Log

L1: la fase existía sin hijas. Se descompuso antes de tocar código, en el orden que exige
la dependencia real: línea base → métrica → experimentos → promoción gobernada.

L2: F19.4 dejó `Evaluate` sin consumidor y lo declaró. T2 lo cablea; queda anotado acá
para que no se pierda entre fases.
