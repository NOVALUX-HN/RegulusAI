# Handoff — estado real al cierre de la noche

Escrito el 2026-11-26. Rama `develop`. Este documento existe porque la sesión
terminó antes de completar F20 y F21, y la única forma honesta de dejarlo es
decirlo con el estado exacto de cada fase.

## Lo que está terminado y verificado

| Fase | Tareas | Estado | Evidencia |
|---|---|---|---|
| F16 · Capability Fidelity | NOV-134..137 | **Done** | 4 Gauntlets 6/6, revisión independiente, `docs/capability-inventory.md` |
| F17 · Recovery Outcome | NOV-138..141 | **Done** | 3 Gauntlets 6/6, `docs/progress-signals.md` |
| F18 · Knowledge Utility | NOV-142..145 | **Done** | 3 Gauntlets 6/6, `docs/knowledge-model-audit.md` |
| F19 · Context & Tool Opt. | NOV-146..149 | **Done** (gate de fase en curso) | `docs/context-tool-baseline.md` |

Deuda de cableado resuelta además: `execbackend`, `recovery.Classify`,
`StrategyOutcome`, y las cuatro funciones de lifecycle de F18 tienen consumidor.

## Lo que NO está hecho

**F20 · Engineering Quality Protocol & Verification Effectiveness** (NOV-132).
Sin empezar. Cuatro hijas: NOV-150 (adoptar REQP en el workflow), NOV-151
(métricas CRAP/Dry y tooling), NOV-152 (efectividad de mutation testing), NOV-153
(efectividad de reviewer y gates).

**F21 · Quality-Adjusted Performance & Governed Evolution** (NOV-133).
Solo el padre: **no tiene hijas**. Necesita descomposición antes de ejecutarse.

## Por qué se detuvo acá

Presupuesto de contexto de la sesión, no falta de trabajo pendiente: F19.4 cerró
justo antes de agotarlo. F20 son cuatro tareas y F21 exige primero un diseño. Cada
una implica leer la issue completa, escribir feature doc, implementar, correr el
paquete, commitear, Gauntlet y revisión independiente. Eso no entraba en lo que
quedaba, y entregar la mitad sin decirlo habría sido el peor resultado posible.

## Cómo se trabajó, para que mañana sea repetible

El patrón que funcionó en F16, F17, F18 y F19, en este orden exacto:

1. **Leer la issue padre y CADA hija completa**, no el resumen. Las cuatro hijas de
   cada fase comparten un bloque de Acceptance criteria genérico idéntico; el
   requisito real vive solo en `## Objetivo`. Cuarta fase consecutiva con ese
   patrón.
2. **Verificar** rama, HEAD, árbol limpio y `origin`.
3. **Feature doc** en `odd/tasks/<fase>.md` con specs numeradas (`S#`) que citan
   el texto del padre **verbatim**, tabla de tareas y sección `## Log`.
4. **Auditoría read-only primero** (T1 de cada fase). Si no hay brecha demostrada,
   se cierra audit-only y no se inventa trabajo.
5. **Implementación mínima**, con tests que intentan romper la garantía.
6. **Verificar**: `bash scripts/verify-wsl.sh <paquetes>` y `go vet`.
7. **Commitear** con mensaje que explique la brecha y la evidencia.
8. **Gauntlet**: `bash scripts/regulus-wsl.sh gate run --quick --yes <tarea>`.
   Tarda 5–9 minutos; correr en background.
9. **Revisión independiente** con `gentle-ai-verify` sobre el entregable. Encontró
   defectos reales en todas las fases, incluido un regreso que introduje en F16.3.
10. **Cerrar en Linear** con comentario que incluye evidencia y límites declarados.

## Trampas ya pagadas — no volver a pagarlas

* **`docdrift`**: un documento que diga "N tools" afirma el total de la superficie.
  Si querés hablar de un subconjunto, nombralo sin número.
* **Pin del dispatch**: agregar un comando de CLI exige subir
  `TestRunDispatchTableCompleteness` (hoy 41) con la razón al lado.
* **`TestNoRawProcessLaunch`**: todo spawn va por `internal/pxexec`.
* **Sin código sin consumidor**: una capacidad sin llamador en producción es deuda.
  Ya pasó tres veces; verificar el consumidor antes de cerrar una tarea.
* **Nunca `git commit --amend`** sobre un commit que ya viajó a `origin`.
* **Backticks en `git commit -m`** son sustitución de comandos: usar
  `git commit -F -` con heredoc `<<'MSG'`.
* **Tokens son proxy**: el host no expone conteos. Cualquier ahorro se etiqueta
  `proxy`/`estimated`/`unknown`, nunca `measured` sin evidencia.

## Comandos de referencia

```bash
# tests (Linux, sin bloqueos de Windows)
bash scripts/verify-wsl.sh ./internal/<paquete>/
# plugin TypeScript
cd .opencode && bun test lib/
# Gauntlet de una tarea
bash scripts/regulus-wsl.sh gate run --quick --yes <fase>/<tarea>
# estado
git log --oneline -5 && git status -sb
```

## Qué haría primero mañana

1. Confirmar el Gauntlet de cierre de F19 (`f19/phase-close`) y cerrar NOV-148/149
   y NOV-131 en Linear si pasó.
2. Leer **NOV-132 completa** y las cuatro hijas completas.
3. Escribir `odd/tasks/engineering-quality-protocol.md` con specs verbatim y las
   cuatro tareas.
4. Empezar por **NOV-150** (adoptar REQP), que es el que condiciona a los otros
   tres.
5. Para F21: primero **descomponer** NOV-133 en 3–4 hijas antes de tocar código.

## Estado del repo al escribir esto

`develop`, árbol limpio, 40 commits por delante de `origin/main`. Nada pusheado
por esta sesión: el push es tuyo, como pediste.
