# knowledge-utility-lifecycle — F18 · Knowledge Utility, Validation & Lifecycle

Linear: NOV-130 (padre), NOV-142..NOV-145 (hijas). Baseline: `develop`, nunca `main`.

Estado de arranque verificado: rama `develop`, `HEAD 136cc28`, árbol limpio, sincronizado con `origin/develop`.

## Specs

S1 — No crear una segunda memoria. Reutilizar Almanac (NOV-22 redacción, NOV-29 concurrencia/idempotencia, NOV-30 backup/restore, NOV-101 cross-agent learning con procedencia). Texto verbatim del padre: "No crear una segunda memoria."

S2 — Gap analysis de los micro-learning records: scope, provenance, evidence, confidence y validation state.

S3 — Promoción basada en evidencia suficiente, repetición independiente o contextual, y revisión de contradicciones.

S4 — Scope-aware retrieval (proyecto, host, entorno, task) con freshness, revalidation y expiry.

S5 — Contradicciones: coexistencia contextual, superseding y revisión. Nunca sobrescritura ciega.

S6 — Medir utilidad del conocimiento (retrieval, reuse, success, reducción de errores y retries) contra un baseline.

S7 — Seguridad: secretos, datos no confiables, instrucciones almacenadas, isolation y permission boundary.

S8 — Lifecycle audit: review, reject, deactivate y rollback; sin duplicados; idempotente.

S9 — Exclusiones verbatim del padre: "No crear store paralelo, no promover observaciones de una sola ejecución como reglas universales, no ejecutar comandos guardados ni otorgar permisos."

S10 — Si no hay brecha demostrada, no se inventa trabajo: se reporta y se propone cierre como audit-only.

S11 — Baseline `develop`. Distinguir host real de fixture y medición de estimación/proxy. Nunca debilitar tests o gates para aparentar éxito.

Texto verbatim del DoD del padre: "Schema y lifecycle gap audit; retrieval tests contextuales; contradiction/freshness tests; isolation/secret adversarial tests; utilidad comparada con baseline; receipts/evidence; regresión; Gauntlet y independent review."

## Tasks

| ID | Linear | Specs | Ruta | Commit |
|----|--------|-------|------|--------|
| T1 | NOV-142 | S1 S2 S10 | inline + explore | `docs: knowledge schema and lifecycle audit (F18.1)` |
| T2 | NOV-143 | S3 S4 S5 S11 | inline | `feat(knowledge): evidence thresholds, freshness and superseding` |
| T3 | NOV-144 | S4 S6 S11 | inline | `feat(knowledge): scope-aware retrieval usefulness evidence` |
| T4 | NOV-145 | S7 S8 S9 | inline | `test(knowledge): isolation, secret and lifecycle adversarial fixtures` |

T1 — auditoría del modelo de micro-learning y de las brechas de promoción, con ubicación en código. Read-only. Su salida condiciona T2–T4: si una brecha no aparece, no se implementa.

T2 — umbrales de promoción, corroboración, confidence, contradicción contextual, expiry/revalidación y superseding.

T3 — retrieval por scope y medición de utilidad contra baseline, con evidencia de reuso y efecto sobre errores/retries.

T4 — adversariales de secretos, prompt injection en conocimiento, contaminación entre proyectos, datos externos y comandos almacenados; validar reject/deactivate/rollback.

## Agent execution protocol (obligatorio, del padre)

* Sólo `develop`; nunca `main` como baseline.
* Confirmar rama, HEAD, árbol y estado remoto antes de empezar.
* Confirmar que cada ruta/API referenciada existe antes de proponer cambios.
* Demostrar la brecha con evidencia; si no existe, no inventar trabajo.
* TDD; mutation tests en admission/isolation cuando sea apropiado.
* Reportar archivos, evidencia, comandos y resultados exactos, límites y riesgos.

## Log

L1: "procede pero siempre revisa lo que tiene cada tarea para conocer a detalle el alcance de cada una de ellas"

L2: Las cuatro hijas comparten el mismo bloque genérico de Acceptance criteria y las mismas Instrucciones de 6 pasos; el requisito real de cada una vive en su `## Objetivo`. Criterio: cada tarea se juzga contra su Objetivo + las specs de este documento, y el template es checklist mínimo, no alcance. Es el tercer fase consecutiva con este patrón (F16, F17, F18).
