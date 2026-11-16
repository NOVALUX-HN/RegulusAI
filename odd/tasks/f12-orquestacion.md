# F12 — Orquestación inteligente y evolución autónoma

Header: milestone F12 (8 issues NOV-96..103, todas Backlog, sin estimates ni comentarios). Se ejecuta después de F11 (todas dependen de F11.x). Cada tarea cierra en Linear como las finalizadas: comentario `## F12.x NOV-xxx — Implementado` + Qué se hizo + Resultado + estado Done. Solo commits (uno por tarea terminada), ningún push (pushea el usuario).

## Specs

- S1 (NOV-96 F12.1 · Adaptive Agent Orchestration y routing explicable):
  - "## Objetivo — Elegir agentes/hosts según tarea, riesgo, capabilities probadas, rendimiento histórico, coste y disponibilidad."
  - "## Implementación — Definir task descriptor (dominio, lenguaje, complejidad, riesgo, restricciones, outputs y criterios); registry de agentes con capabilities/evidencia/límites; empezar con políticas deterministas y scoring explicable; decidir uno o varios agentes y justificar fallback/no delegación; aplicar budgets de tiempo/tokens/coste y límites de delegación; añadir dry-run; comparar con routing fijo."
  - "## Aceptación — Decisión reproducible y explicable; nunca asignar capability no verificada; tareas sencillas pueden usar un agente y críticas justificar revisión; fallback respeta permisos; evaluación mide calidad, latencia y coste y puede concluir que routing dinámico no aporta."
  - "## Dependencias — F11.1, F11.2, F11.4, F11.5, F11.7 y F11.8."

- S2 (NOV-97 F12.2 · Parallel Engineering con worktrees y merge seguro):
  - "## Objetivo — Paralelizar trabajo independiente reduciendo tiempo de pared sin conflictos ni pérdida de control."
  - "## Implementación — Descomponer tareas en DAG; detectar dependencias/colisiones y serializar cuando corresponda; crear worktree/branch/contexto/permiso aislado por worker; scheduler con concurrencia, budgets, cancelación y backpressure; integrar mediante merge controlado y gates; resolver conflictos con integrador y revisión; limpiar worktrees huérfanos."
  - "## Aceptación — Dos tareas independientes trabajan en paralelo y se integran; tareas conflictivas se serializan/coordinan; fallo de worker no corrompe otros; merge final pasa gates y conserva trazabilidad; reporte muestra tiempo pared y coste agregado."
  - "## Dependencias — F11.1, F11.3, F11.4, F11.8 y F12.1."

- S3 (NOV-98 F12.3 · Predictive Context Engine y selección de contexto):
  - "## Objetivo — Entregar a cada agente el contexto mínimo suficiente, pertinente y actualizado."
  - "## Implementación — Inventariar código, símbolos, dependencias, docs, ADRs, issues y memoria; rankear relevancia explicable por tarea/perfil; detectar duplicados, obsolescencia, contradicciones y fuentes no confiables; entregar capas imprescindible/bajo demanda/opcional; medir tokens y calidad antes/después; respetar ACLs/secretos; fallback conservador."
  - "## Aceptación — Cada fragmento tiene fuente, versión/fecha y razón; no se omite contexto crítico en corpus; baja redundancia sin degradar calidad; se detecta obsolescencia/contradicción; tests cubren retrieval irrelevante, prompt injection y permisos."
  - "## Dependencias — F11.1, F11.5 y F12.1."

- S4 (NOV-99 F12.4 · Continuous Quality Intelligence y detección de regresiones):
  - "## Objetivo — Detectar degradación de calidad, rendimiento, coste y compatibilidad a partir de ejecuciones reales."
  - "## Implementación — Baselines por agente/modelo/host/tarea/versión; ingerir results/traces/gates/evals con procedencia; detectar cambios en éxito, retries, latencia, tokens, errores y cobertura; usar umbrales/métodos adecuados al N; producir diagnóstico con casos/evidencia; proponer investigación, nunca modificar producción automáticamente."
  - "## Aceptación — Regresión inyectada detectada y vinculada a cambios/runs; alerta muestra baseline, ventana, N, magnitud y limitaciones; segmentación controla falsos positivos; reproducción desde evidencia versionada."
  - "## Dependencias — F11.1, F11.3, F11.4 y F11.5."

- S5 (NOV-100 F12.5 · Autonomous Recovery, checkpoints y reanudación idempotente):
  - "## Objetivo — Recuperar ejecuciones interrumpidas sin reiniciar todo ni duplicar efectos secundarios."
  - "## Implementación — Definir estados durables/checkpoints por tarea y agente; persistir inputs, outputs, artifacts, permisos, versiones y último estado confirmado; reanudación, retry budgets, backoff, timeout y circuit breakers; clasificar errores; reasignar solo si capabilities/estado lo permiten; idempotency keys o compensaciones; cancelación, cleanup y rollback parcial."
  - "## Aceptación — Crash simulado en transiciones críticas se reanuda sin duplicar efectos; retries acotados; reasignación conserva contexto/evidencia y permisos; estado final auditado; tests cubren crash, red, host offline, timeout y operación parcial."
  - "## Dependencias — F11.1, F11.8, F12.1 y F12.2."

- S6 (NOV-101 F12.6 · Cross-Agent Learning y memoria con procedencia):
  - "## Objetivo — Compartir conocimiento útil entre agentes sin compartir conversaciones indiscriminadamente ni convertir hipótesis en hechos."
  - "## Implementación — Tipos de memoria: solución verificada, patrón, decisión, error, workaround e hipótesis. Guardar fuente, autor/agente, fecha, versión, confianza, alcance y evidencia. Recuperar por relevancia; invalidar conocimiento obsoleto; separar memoria privada de tarea/proyecto/compartible; deduplicar, resolver contradicciones y aplicar ACLs."
  - "## Aceptación — “Verificado” exige pruebas/evidencia; hipótesis nunca se muestran como hechos; retrieval respeta permisos; medir recall útil, precisión, obsolescencia y ruido; existe invalidación y auditoría."
  - "## Dependencias — F11.1, F11.8 y F12.3."

- S7 (NOV-102 F12.7 · Governed Self-Evolution y promoción segura de mejoras):
  - "## Objetivo — Cerrar ciclo de mejora: detectar debilidad, formular hipótesis, generar candidatos y promover solo con evidencia y gobernanza."
  - "## Implementación — Ingerir señales de F12.4, evals y memoria; definir hipótesis/superficie reversible; generar candidatos versionados con K acotado; comparar baseline/candidato con repeticiones y holdout gobernado; evaluar calidad, latencia, coste, seguridad y regresiones; separar proposer/evaluator/approver/deployer; exigir aprobación humana para Core, permisos, políticas y evaluadores; canary, monitorización y rollback con condiciones preaprobadas."
  - "## Aceptación — Ciclo reproducible; promoción bloqueada ante evidencia insuficiente/regresión; approvals ligados a hashes/policies; rollback probado; holdout intacto; revisión adversarial independiente; ningún agente aprueba su propio cambio crítico."
  - "## Dependencias — F11.1, F11.3, F11.5, F11.6, F11.8, F12.4 y F12.6."

- S8 (NOV-103 F12.8 · Impact Analysis y grafo de dependencias del repositorio):
  - "## Objetivo — Predecir componentes, contratos y pruebas afectados antes de cambiar código."
  - "## Implementación — Inventariar módulos, paquetes, APIs, config, workflows y dependencias; construir grafo incremental con aristas verificables; mapear diff a consumidores, rutas críticas y tests; calcular riesgo explicable y seleccionar suite enfocada; detectar ciclos/dependencias ocultas/cambios incompatibles; evaluar CodeGraph u otras herramientas solo con benchmark."
  - "## Aceptación — Cambio conocido identifica consumidores/tests; cada arista tiene fuente y commit/fecha; medir precisión/recall de selección frente a full suite; ante incertidumbre ampliar selección; no omitir pruebas críticas por inferencia débil; decisión de herramienta externa basada en coste/beneficio."
  - "## Dependencias — F11.1, F11.3, F12.3 y F12.4."

## Tasks

| ID | Spec | NOV | Deps | Route | Commit |
|----|------|-----|------|-------|--------|
| T1 | S1 | NOV-96 F12.1 | F11.2,F11.4,F11.5,F11.7,F11.8 | worker | 42e73f1 |
| T2 | S2 | NOV-97 F12.2 | F11.1,F11.3,F11.4,F11.8,T1 | worker | dd1566d |
| T3 | S3 | NOV-98 F12.3 | F11.1,F11.5,T1 | worker | 9d17d8e |
| T4 | S4 | NOV-99 F12.4 | F11.1,F11.3,F11.4,F11.5 | worker | — |
| T5 | S5 | NOV-100 F12.5 | F11.1,F11.8,T1,T2 | worker | — |
| T6 | S6 | NOV-101 F12.6 | F11.1,F11.8,T3 | worker | — |
| T7 | S7 | NOV-102 F12.7 | F11.1,F11.3,F11.5,F11.6,F11.8,T4,T6 | worker | — |
| T8 | S8 | NOV-103 F12.8 | F11.1,F11.3,T3,T4 | worker | — |

## Log

- L1 (usuario, verbatim): "arranca, pero siempre en cada tarea trae toda la información de linear de cada tarea ya que explica que es lo que se desea. haz esto hasta finalizar todas las tareas y solo haz commits de cada tarea terminada, no hagas ningún push"
- L2: corre después de F11 (todas dependen de F11.x). Orden numérico F12.1→F12.8 (la .7 ya exige 7 dependencias incl. F12.4/F12.6).
- L3 (2026-11-16): F11 completa 9/9 (commits c59c9a7..015cd1f, Linear Done). Arranca F12.
- L4 (2026-11-16, T1 done): NOV-96 F12.1 implementado (orquestador: descriptor + registry + scoring explicable + dry-run + evaluate, 10+7 tests; RED real de gate crítico corregido) commit 42e73f1; pin de dispatch actualizado por parent; Linear con evidencia y Done; CLI BLOCKED local (verifica CI).
- L5 (2026-11-16, T2 done): NOV-97 F12.2 implementado (DAG + serialización por colisión + scheduler por niveles + worktrees reales + merge controlado + gate + orphans, tests verdes local incl. overlap real) commit dd1566d; Linear con evidencia y Done.
- L6 (2026-11-16, T3 done): NOV-98 F12.3 implementado (inventario + ranking explicable + detección dup/obsolescencia/contradicción/inyección + capas + ACLs + fallback, 9+5 tests; 2 RED reales corregidos) commit 9d17d8e; pin de dispatch actualizado por parent; Linear con evidencia y Done.
