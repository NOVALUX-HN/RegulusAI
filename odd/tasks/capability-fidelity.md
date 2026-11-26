# capability-fidelity — F16 · Capability Fidelity & Adapter Gap Closure

Linear: NOV-128 (padre), NOV-134..NOV-137 (hijas). Baseline: `develop`, nunca `main`.

Estado de arranque verificado: rama `develop`, `HEAD = 5d57337`, árbol limpio, 15 commits por delante de `origin/develop` (no pusheado).

## Specs

S1 — No reconstruir lo ya hecho. F6 (NOV-48–55) y F11 (NOV-93/94) ya entregaron contrato neutral de capacidades, adapters, Skill Registry, conformidad y política server-side. Reutilizar, nunca duplicar.

S2 — Auditoría con evidencia: capacidad declarada vs capacidad real/observada, con ubicación en código, pruebas, evidencia, limitación y brecha real por host.

S3 — Matriz `capability / adapter / versión / evidencia / limitaciones` como artefacto versionado, no como prosa.

S4 — Vocabulario uniforme de estados: `unavailable`, `partial`, `inferred`, `verified`, `degraded`, con versión, evidencia y restricciones.

S5 — Contract tests y fixtures multi-host para las capacidades críticas, incluyendo trust boundaries, capability mismatch, evento/resultado malformado y errores de lifecycle.

S6 — El core no depende de ningún host y no colapsa por una capacidad parcial: fallback controlado cuando falta, cambia o falla.

S7 — No asumir capacidades de un host; no inventar capacidades; nunca presentar un estimado, proxy o fixture como dato real de host.

S8 — Si no hay brecha demostrada, no se inventa trabajo: se reporta y se propone cierre como audit-only.

S9 — Nunca debilitar, saltear ni falsificar tests, gates o controles para que una tarea parezca completa. Un criterio no demostrado queda abierto.

S10 — Reversalibilidad: cambios incrementales, pequeños y revisables; sin refactors amplios ni dependencias especulativas.

Texto verbatim de la exclusión del padre: "No reconstruir Codex, Claude Code, Devin u OpenCode adapters; no prometer paridad; no añadir hosts sin necesidad demostrada; no inventar capacidades ni usar datos estimados como reales."

## Tasks

| ID | Linear | Specs | Ruta | Commit |
|----|--------|-------|------|--------|
| T1 | NOV-134 | S1 S2 S3 S7 S8 | inline + explore | `docs: capability inventory (F16.1 audit)` |
| T2 | NOV-135 | S4 S7 S9 S10 | inline | `feat(capability): normalized capability states with evidence` |
| T3 | NOV-136 | S6 S9 S10 | inline | `feat(capability): degraded fallback and error semantics` |
| T4 | NOV-137 | S5 S9 S10 | inline | `test(capability): cross-host contract and adversarial fixtures` |

T1 — inventario por host/capability con ubicación en código, pruebas, evidencia, limitaciones y brecha real. Read-only. Su salida condiciona T2–T4: si una brecha no aparece, no se implementa.

T2 — completar únicamente los campos/estados faltantes del registry y contratos para declarar los cinco estados con versión, evidencia y restricciones.

T3 — implementar o corregir el fallback controlado; el core no debe colapsar por capacidad parcial.

T4 — expandir fixtures y contract tests de interfaces/adapters críticos con casos adversariales.

## Agent execution protocol (obligatorio, del padre)

* Trabajar sólo sobre `develop`; nunca `main` como baseline.
* Confirmar rama, HEAD, árbol y estado remoto antes de empezar.
* Confirmar que cada ruta/API referenciada existe antes de proponer cambios.
* Identificar la brecha concreta con evidencia; si no existe, no inventar trabajo.
* Distinguir ejecución en host real de fixture/mock, y medición de estimación/proxy.
* Reportar archivos, evidencia, comandos y resultados exactos, límites y riesgos.

## Log

L1: "primero haz commit de lo que tenemos y luego comienza con F16 pero debes de leer cada tarea detalladamente porque cada una tiene informacion importante que considerar, debes leerla paso a paso y cumplir con lo que se pide"

L2: Las cuatro hijas comparten un bloque de Acceptance criteria genérico idéntico; el requisito real de cada una vive sólo en su Objetivo. Decisión: cada tarea se juzga contra su Objetivo + las specs de este documento, y el template se trata como checklist mínimo, no como alcance.

L3: NOV-134 decía "auditar main" en una revisión anterior y ahora dice "auditar develop"; el protocolo del padre (develop-only) y el texto vigente coinciden. Sin contradicción pendiente.
