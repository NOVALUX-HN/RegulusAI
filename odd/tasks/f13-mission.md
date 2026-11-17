# F13 — Autonomous Mission Execution Engine

Header: milestone F13 (8 issues NOV-104..111, todas Backlog salvo las que se vayan cerrando, sin estimates ni comentarios). Reglas transversales de la fase (verbatim de cada issue): integrar F12 sin duplicar; inspeccionar rama real y reutilizar orchestrate, context, quality, recovery, almanac/learning, evolve, impact, observability/traces, gates y permisos MCP; Go Core dueño del estado; evidencia con comandos exactos, sin convertir BLOCKED/SKIPPED/UNKNOWN en PASS, sin marcar Done sin criterios; multi-host seguro; no tocar holdouts. Cada tarea cierra en Linear como las finalizadas: comentario `## F13.x NOV-xxx — Implementado` + Qué se hizo + Resultado + estado Done. Solo commits (uno por tarea terminada), ningún push (pushea el usuario).

## Specs

- S1 (NOV-104 F13.1 · Mission Engine y ciclo durable de misiones):
  - "## Objetivo — Crear la entidad y el motor central que representa, gobierna y persiste una misión de ingeniería de principio a fin. Será la fuente de verdad del estado global, sin reemplazar los estados especializados de F12.2/F12.5."
  - "## Implementación — * Definir modelo Mission con ID estable, objetivo, descripción, repositorio/workspace, restricciones, criterios de aceptación, nivel de riesgo, presupuesto, creador, timestamps y versión de esquema. * Definir estados y transiciones explícitas: created, analyzing, planning, awaiting_approval, executing, integrating, validating, recovering, completed, partially_completed, blocked, failed y cancelled. * Implementar máquina de estados con transiciones permitidas, invariantes y rechazo de transiciones ilegales. * Persistencia durable, escrituras atómicas, integridad/checksum y migración versionada; reutilizar almacenamiento existente cuando sea apropiado. * Correlacionar mission_id con run_id, task_id, agent_id, worktree, commits, traces y receipts. * Checkpoint de misión, carga de estado, pause/resume/cancel y recuperación después de reinicio. * API interna y CLI mínima para create/show/list/status/pause/resume/cancel; evitar ampliar CLI si una interfaz existente cubre el caso. * Eventos de ciclo de vida auditables e idempotentes. * Política de concurrencia para impedir dos coordinadores mutando la misma misión."
  - "## Criterios de aceptación — * Una misión puede crearse, consultarse y transitar únicamente por estados válidos. * Dos procesos concurrentes no corrompen ni sobrescriben silenciosamente el estado. * Simular crash entre cada transición crítica y recuperar el último estado confirmado. * Reanudar no duplica tareas ni efectos secundarios. * Migraciones y datos corruptos se manejan explícitamente, sin pérdida silenciosa. * Cada evento y artefacto se puede correlacionar con mission_id. * Estados terminales no pueden reabrirse salvo operación explícita y auditada."
  - "## Pruebas requeridas — Unitarias de máquina de estados, persistencia, migración, concurrencia y validación; integración multi-proceso; crash/restart; cancelación y reanudación; compatibilidad de CLI/API."
  - "## Entregables — Código, documentación del modelo y transiciones, CLI/API, tests y evidencia de recuperación."

(Se añaden S2-S8 con el cuerpo verbatim de NOV-105..111 al arrancar cada tarea.)

## Tasks

| ID | Spec | NOV | Deps | Route | Commit |
|----|------|-----|------|-------|--------|
| T1 | S1 | NOV-104 F13.1 | F11.1,F12.2,F12.5 | worker | 160f0e1 |
| T2 | S2 | NOV-105 F13.2 | T1 + F12.3,F12.8,F12.1 | worker | — |
| T3 | S3 | NOV-106 F13.3 | T1 + F12.1,F12.2 | worker | — |
| T4 | S4 | NOV-107 F13.4 | F11.1,F11.3 + F12.8,F12.4 | worker | — |
| T5 | S5 | NOV-108 F13.5 | F12.5 + feeds F12.6 | worker | — |
| T6 | S6 | NOV-109 F13.6 | F11.8 + policies | worker | — |
| T7 | S7 | NOV-110 F13.7 | F12.4 + gates | worker | — |
| T8 | S8 | NOV-111 F13.8 | todas | worker | — |

## Log

- L1 (usuario, verbatim): "ya se hizo push de forma manual. revisa detenidamente cada tarea y cumple con lo que se pide"
- L2: F13 nueva en Linear (8 issues NOV-104..111, Urgent salvo NOV-110 High, sin estimates ni comentarios). Previo obligatorio cumplido: push verificado en origen, CI del árbol F11+F12 diagnosticado y reparado localmente (secret scan exit 0, 2 tests Windows PASS, size/vet/build verdes; commit 397a93f ahead 1 sin push) antes de arrancar F13.1 como exige la spec.
- L3 (2026-11-17, T1 done): NOV-104 F13.1 implementado (motor 13 estados + store durable + migración + concurrencia optimista + ledger exactly-once + CLI mínima, tests incl. multi-proceso real) commit 160f0e1; pin de dispatch actualizado por parent; Linear con evidencia y Done.
