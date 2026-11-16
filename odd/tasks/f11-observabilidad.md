# F11 — Observabilidad, validación y excelencia operativa

Header: milestone F11 (9 issues NOV-87..95, todas Backlog, sin estimates ni comentarios). Orden de ejecución: T1 base (F11.1), T2 urgent (F11.8, depende F11.1), T3-T8 en orden numérico, T9 cierra F11. Cada tarea cierra en Linear como las finalizadas: comentario `## F11.x NOV-xx — Implementado` + Qué se hizo + Resultado + estado Done. Solo commits (uno por tarea terminada), ningún push (pushea el usuario).

## Specs

- S1 (NOV-87 F11.1 · Instrumentación y trazabilidad individual de agentes):
  - "## Objetivo — Reconstruir cualquier ejecución por agente, tarea y delegación, desde el inicio hasta el resultado."
  - "## Implementación — * Definir IDs correlacionables: run_id, task_id, agent_execution_id, delegation_id, session_id y event_id; propagarlos por Core, plugin y adaptadores. * Crear eventos estructurados versionados para start/end, contexto metadata, tool call/result, delegación, retry, timeout, gate, artifact, decisión y error. * Registrar host/model/version, timestamps UTC y monotónicos, duración, estado y relaciones padre-hijo. * Persistir incrementalmente, con límites, retención y redacción previa de secretos. No guardar transcripts completos por defecto. * Añadir CLI/API para consultar timeline por run, tarea o agente y reconstruir la ejecución tras un crash."
  - "## Aceptación — * Un run integrado se reconstruye desde eventos persistidos; delegaciones atribuibles a origen/destino. * Eventos tienen schema_version, IDs y estados terminales inequívocos. * Tests cubren duplicados, desorden, concurrencia, crash, datos malformados y redacción. * Documentar schema, migraciones y comandos; adjuntar trace/receipt anonimizado y hashes."
  - "## No hacer — No confundir logs libres con trazabilidad estructurada; no implementar dashboard visual aquí."

- S2 (NOV-88 F11.2 · Agent Lab: ejecución aislada y comparación de agentes):
  - "## Objetivo — Ejecutar cualquier agente individualmente y comparar agentes/modelos con la misma tarea sin lanzar el corpus completo."
  - "## Implementación — * Inventariar agentes, responsabilidad, prompt/skill, herramientas, host y entradas. * Crear modo individual seleccionable por agente, fixture, host/modelo y presupuesto. * Añadir modo comparativo con misma tarea, fixture, criterios y límites; modo integrado para seguir el flujo completo. * Aislar workspace, outputs, sesiones y permisos por ejecución. * Reportar instrucciones/metadata recibida, eventos, herramientas, delegaciones, duración, tokens con procedencia, reintentos, errores, cambios y resultado. * Exportar y repetir configuración; clasificar pass/fail/timeout/infra-error/inconclusive/not-run."
  - "## Aceptación — Cada agente registrado tiene ejecución aislada reproducible; al menos dos agentes pueden compararse; ningún modo exige corpus completo; resultados enlazan trazas F11.1; tests validan aislamiento, replay y cancelación."
  - "## Evidencia — Comandos, fixtures, configuración, reportes y traces de dos agentes. No implementar routing automático (F12.1)."

- S3 (NOV-89 F11.3 · Evaluación escalonada L0–L3 y ejecución selectiva del corpus):
  - "## Objetivo — Reducir tiempo de feedback sin eliminar tareas, bajar dificultad ni contaminar holdout."
  - "## Implementación — * L0 smoke por cambio; L1 focused por agente/capacidad; L2 regression representativa; L3 full corpus + holdout. * Seleccionar pruebas por componentes/capacidades afectadas y explicar inclusión/omisión. * Preservar manifiestos, fixtures fijados por commit, checks ocultos, splits y anti-leakage. * Medir preparación, cola, inferencia, herramientas, evaluación y teardown por separado. * Presupuestos/timeouts por tarea, agente y run; clasificar timeout/infra/no-evaluable correctamente. * Paralelizar solo tareas independientes con aislamiento, concurrencia limitada y respeto de rate limits. * Exigir L3 periódico y previo a releases; holdout sellado."
  - "## Aceptación — Niveles y comandos documentados; cambios pequeños no disparan L3 por defecto; L2/L3 preservan cobertura; reporte indica pruebas ejecutadas y omitidas con razón; se demuestra menor latencia de feedback sin degradar calidad; tests cubren leakage y selección incorrecta."
  - "## Evidencia — Manifiestos, hashes, comparación de tiempo de pared y agregado. No recortar el corpus."

- S4 (NOV-90 F11.4 · Diagnóstico de rendimiento y cuellos de botella):
  - "## Objetivo — Identificar qué agente, etapa, herramienta o delegación causa lentitud, reintentos y trabajo improductivo."
  - "## Implementación — Consumir trazas F11.1 y Agent Lab; medir duración por agente/tarea/etapa, pared vs tiempo agregado, cola/espera, latencia de tools, retries, cancelaciones y fallos. Calcular mediana y p90/p95 solo con N suficiente. Detectar loops, duplicación, herramientas lentas y delegaciones sin contribución. Añadir diagnósticos explicables y umbrales configurables; no autoajustar parámetros."
  - "## Aceptación — Una ejecución lenta identifica etapa/agente dominante con evidencia enlazada; métricas distinguen pared/agregado; alertas muestran muestra y umbral; no se afirman anomalías con N insuficiente; tests cubren loops, outliers, timeouts y datos parciales."
  - "## Evidencia — Informe reproducible de una ejecución lenta real o escenario controlado, con baseline. Depende de F11.1 y F11.2."

- S5 (NOV-91 F11.5 · Contabilidad de tokens y coste por agente/run/host):
  - "## Objetivo — Atribuir tokens/costes con procedencia verificable, sin confundir mediciones, proxies y desconocidos."
  - "## Implementación — Auditar fuentes OpenCode, Claude Code, Codex y Devin; implementar readers de solo lectura para formatos verificados; correlacionar usage con run/task/agent/session/host; normalizar input/output/reasoning/cache sin perder datos originales; etiquetar measured-host, measured-local, estimated, proxy y unknown; calcular coste solo con tarifa/version/fecha explícitas; redactar secretos y añadir fixtures hostiles/malformadas."
  - "## Aceptación — Al menos un host aporta tokens reales end-to-end por agente/tarea; otros declaran estado y razón verificados; totales concilian con fuente primaria; CLI/reportes exponen procedencia; unknown nunca equivale a cero; tests cubren límites, overflow y duplicados."
  - "## Evidencia — Transcript/export anonimizado, versión host, comandos, cálculo y receipt. Depende de F11.1. Reconciliar NOV-73 sin duplicar trabajo."

- S6 (NOV-92 F11.6 · Validación reproducible de Evolution y ciclo de mejora):
  - "## Objetivo — Demostrar que Evolution mejora calidad, latencia o coste con evaluación independiente y rollback."
  - "## Implementación — Revisar NOV-71/NOV-80 y su evidencia real; declarar hipótesis, superficie reversible, baseline, métricas y presupuesto antes de experimentar; ejecutar candidatos con seeds/repeticiones/modelos comparables; registrar descartes y controles positivo/negativo; mantener holdout sellado hasta evaluación final; exigir evaluador independiente y aprobación humana; probar canary/rollback."
  - "## Aceptación — Informe reproducible con manifest, commits, hashes, comandos, traces y receipts; calidad/latencia/tokens separados con N y variabilidad; al menos un rechazo/rollback real; decisión sobre cambios heredados explícita; rollback probado; no adoptar regresiones ni afirmar significancia sin muestra suficiente."
  - "## Dependencias — F11.1, F11.3 y F11.5. Reconciliar NOV-80/NOV-71; no asumir que Done demuestra los criterios."

- S7 (NOV-93 F11.7 · Conformidad multi-host y matriz de capacidades):
  - "## Objetivo — Sustituir afirmaciones generales de compatibilidad por journeys ejecutados y capabilities verificadas."
  - "## Implementación — Definir capability manifest por host (tools, hooks, subagents, MCP, sessions, tokens, filesystem, permisos y límites); journeys reales OpenCode/Codex/Claude y estrategia realista Devin; CI con credenciales y distinción explícita entre probe y journey; registrar versiones, config no sensible, capabilities, comandos y artifacts redactados; definir degradación controlada. Revisar NOV-76/NOV-69 para no duplicar."
  - "## Aceptación — Matriz por capacidad: probada/no soportada/no ejecutada/desconocida; al menos dos hosts con journeys E2E repetibles en CI; workflow nunca etiqueta probe como journey; secretos fuera de artifacts; evidencia y versiones enlazadas."
  - "## Dependencias — F11.1 y reconciliación NOV-76/NOV-69."

- S8 (NOV-94 F11.8 · Política de capacidades y autorización server-side para MCP) [URGENT]:
  - "## Objetivo — Hacer que Go Core/servidor aplique permisos; no depender solo del prompt o del cliente MCP."
  - "## Implementación — Inventariar tools y clasificar lectura, escritura, ejecución, destructivas, secretos, memoria/evidencia y cambios al harness. Definir scopes mínimos y deny-by-default para operaciones sensibles. Enforzar antes de efectos secundarios. Separar permisos de agente/host/usuario/workspace y evitar elevación por delegación. Registrar policy version y decisión sin secretos. Probar prompt injection, tool spoofing, confused deputy, path traversal, replay y escalación."
  - "## Aceptación — Ninguna operación sensible se ejecuta sin autorización server-side; delegaciones solo restringen permisos; decisiones auditadas; suite adversarial cubre permitido/denegado; migración compatible documentada."
  - "## Evidencia — Threat model, matriz tool→capability→policy, tests y receipts. Depende de F11.1."

- S9 (NOV-95 F11.9 · Release reproducible, mantenimiento y documentación canónica):
  - "## Objetivo — Cerrar F11 con distribución verificable y documentación coherente con el sistema real."
  - "## Implementación — Revisar PRs/dependencias y actualizar solo con pruebas de compatibilidad; ejecutar tag→builds→checksums→attestations→publicación→instalación limpia; verificar permisos mínimos de workflows y actions fijadas por SHA; documentar arquitectura actual, contratos, hosts, agentes, eventos, benchmarks y operación; mantener PRD histórico como tal y enlazar ADRs actuales; reconciliar claims de soporte y Done con evidencia."
  - "## Aceptación — Al menos una release verificable desde entorno limpio; checksums/attestations comprobados independientemente; dependencias resueltas o justificadas; docs alineadas con código; no se declara soporte sin journey/evidencia."
  - "## Evidencia — Release, CI logs, verificación"

## Tasks

| ID | Spec | NOV | Deps | Route | Commit |
|----|------|-----|------|-------|--------|
| T1 | S1 | NOV-87 F11.1 | — | worker | — |
| T2 | S8 | NOV-94 F11.8 | T1 | worker | — |
| T3 | S2 | NOV-88 F11.2 | T1 | worker | — |
| T4 | S3 | NOV-89 F11.3 | T1 | worker | — |
| T5 | S4 | NOV-90 F11.4 | T1,T3 | worker | — |
| T6 | S5 | NOV-91 F11.5 | T1 | worker | — |
| T7 | S6 | NOV-92 F11.6 | T1,T4,T6 | worker | — |
| T8 | S7 | NOV-93 F11.7 | T1 | worker | — |
| T9 | S9 | NOV-95 F11.9 | T1-T8 | worker | — |

## Log

- L1 (usuario, verbatim): "arranca, pero siempre en cada tarea trae toda la información de linear de cada tarea ya que explica que es lo que se desea. haz esto hasta finalizar todas las tareas y solo haz commits de cada tarea terminada, no hagas ningún push"
- L2: F11+F12 nuevas en Linear (17 issues Backlog, sin estimates ni comentarios, prioridades High salvo NOV-94 Urgent y NOV-95/101/103 Medium). F9/F10 100% Done. Orden: F11.1 base primero, F11.8 urgent segunda (depende F11.1), resto numérico, F11.9 cierra; luego F12 en orden (F12.1 exige F11.2/F11.4/F11.5/F11.7/F11.8).
