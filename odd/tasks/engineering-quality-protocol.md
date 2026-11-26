# engineering-quality-protocol — F20 · Engineering Quality Protocol & Verification Effectiveness

Linear: NOV-132 (padre), NOV-150..NOV-153 (hijas). Baseline: `develop`, nunca `main`.

Leída la fase padre y **cada hija completa**. Estado de arranque: rama `develop`,
árbol limpio, sin empezar.

## Specs

S1 — Aplicar un REQP transversal **sin crear un nuevo reviewer ni un stack de
tooling nuevo**. Texto verbatim del padre: "sin crear un nuevo reviewer/tooling
stack". Reutilizar reviewer, audit, gates, coverage, mutation, fuzzing, CI y
métricas de calidad existentes (NOV-27, NOV-28, NOV-41, NOV-65, NOV-67, NOV-84,
NOV-83, F11 y el Gauntlet actual).

S2 — REQP operativo y adaptado a riesgo y lenguaje, con excepciones justificadas:
subtract, premise, root cause, TDD cuando sea viable, unidades pequeñas,
behavior testing, boundaries, idempotency, evidence, regresión, rollback.

S3 — Tooling decidido por evidencia: compatibilidad, licencia, mantenimiento,
seguridad y rendimiento. Preferir lo nativo existente y **no integrar si el coste
no se justifica**. Cada decisión queda como decision record.

S4 — Hotspots de calidad y risk scoring; **nunca métricas vanity ni cobertura
uniforme ciega**.

S5 — Mutation testing selectivo en paquetes críticos, con sobrevivientes
clasificados y triaje (gap de test vs coste).

S6 — Duplicación estructural, complejidad y dead code: **report-only** al inicio,
remediación proporcional.

S7 — Gates y reviewer verifican diff, tests, invariantes, evidencia, regresiones y
claims; no se vuelven arbitrarios.

S8 — Fixtures de efectividad de reviewer y gates, incluyendo fallos adversariales.

S9 — Texto verbatim de las exclusiones: "No exigir mutation/coverage universales;
no refactor masivo; no instalar UB Mode como dependencia runtime; no aceptar
metrics vanity."

S10 — DoD verbatim: "REQP documentado/integrado; tooling decision records; métricas
con provenance; mutation outcomes en critical packages; auditoría effectiveness;
security/false positive checks; baseline comparison; Gauntlet, regression y
independent review."

S11 — Si no hay brecha demostrada, no se inventa trabajo: se reporta y se propone
cierre como audit-only.

## Tasks

| ID | Linear | Specs | Qué pide |
|----|--------|-------|----------|
| T1 | NOV-150 | S1 S2 S11 | Auditar Sextant/BUILD/PROVE/LEARN, reviewer, audit, gates y DoD; integrar REQP |
| T2 | NOV-151 | S3 S4 S6 | Evaluar métricas CRAP/duplicación/dead code y su tooling |
| T3 | NOV-152 | S5 S10 | Efectividad de mutation testing en rutas críticas |
| T4 | NOV-153 | S7 S8 | Efectividad de reviewer, gates y verificación |

T1 abre la fase porque condiciona a los otros tres: si REQP no está integrado en el
flujo, medir tooling (T2), mutation (T3) o reviewer (T4) no tiene dónde engancharse.

## Advertencia de alcance

La fase pide "evolucionar" siete superficies existentes. El riesgo real es
convertirla en un refactor masivo, que las exclusiones prohíben. Cada tarea debe
demostrar su brecha con evidencia antes de tocar código, y cerrar audit-only si no
la hay.

## Patrón de trabajo (el que funcionó en F16–F19)

1. Leer la issue padre y **cada** hija completa; el requisito real vive en
   `## Objetivo` (las cuatro comparten un bloque genérico de Acceptance criteria).
2. Verificar rama, HEAD, árbol y `origin`.
3. Auditoría read-only primero.
4. Implementación mínima con tests que intentan romper la garantía.
5. `bash scripts/verify-wsl.sh <paquetes>` + `go vet`.
6. Commit con la brecha y la evidencia en el mensaje.
7. `bash scripts/regulus-wsl.sh gate run --quick --yes <fase>/<tarea>` (5–9 min,
   en background).
8. Revisión independiente con `gentle-ai-verify`; en F16–F19 encontró defectos
   reales **todas las veces**, incluido un regreso propio.
9. Cerrar en Linear con evidencia y límites declarados.

## Trampas ya pagadas

* `docdrift`: un documento que diga "N tools" afirma el total de la superficie.
* Pin del dispatch: agregar un comando exige subir `TestRunDispatchTableCompleteness`
  (hoy 41) con la razón al lado.
* `TestNoRawProcessLaunch`: todo spawn por `internal/pxexec`.
* Sin código sin consumidor en producción.
* Nunca `--amend` sobre un commit que ya está en `origin`.
* Backticks en `git commit -m` son sustitución: usar `git commit -F -` con `<<'MSG'`.
* Tokens son proxy: etiquetar `proxy`/`estimated`/`unknown`, nunca `measured` sin
  evidencia.

## Log

L1: "me ire a dormir, así que necesito que no solo finalices el f19 completo, sino
que al finalizar traigas las siguientes fases y analices cada issue como lo hemos
hecho con su informacion completa de cada uno y las hagas hasta finalizar todas las
fases que creamos con sus test funcionando a cabalidad."

L2: F19 se cerró completa (4 tareas, Gauntlet de fase 6/6). F20 quedó **analizada
pero no implementada**: el presupuesto de contexto de la sesión se agotó justo
después. Este documento es el análisis que pediste, listo para ejecutar.

L3: NOV-151 ya no dice "Crapper/Dryer": el título es ahora "Evaluate CRAP,
duplication and dead-code metrics/tooling". La corrección que pedí en el comentario
de mejoras quedó aplicada, así que esa parte del hallazgo está cerrada.
