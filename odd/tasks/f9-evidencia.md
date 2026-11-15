# F9 — Evidencia medida y cierre verificable

Header: programa Regulus-AI, milestone F9 (sortOrder 8197, 0%). 9 issues
en orden topologico Pasos 52-60. Cada tarea cierra en Linear como las
finalizadas: comentario `## T<n> NOV-xx — Implementado` + Que se hizo +
Resultado (comandos, exits, commits, receipts) + `task close --issue`;
P0 exige veredicto independiente (no auto-cierre).

## Specs

- S1 (NOV-72 Paso 52): workflows ejecutables: `runs-on: ubuntu-24-latest`
  no existe como label (docs GitHub: `ubuntu-latest` o `ubuntu-24.04`);
  el repo pinea `ubuntu-24.04`. Incluye evidencia de conformidad de
  hosts y release de prueba con tag real + version 0.15.
- S2 (NOV-73 Paso 53): tokens reales de punta a punta en al menos un
  host (hoy todo `unknown`).
- S3 (NOV-76 Paso 54): journeys reales en al menos dos hosts
  (host-probado) con evidencia enlazada.
- S4 (NOV-77 Paso 55): corpus de evals ampliado, base por debajo del
  100%, evidencia reproducible.
- S5 (NOV-74 Paso 56): Done = criterios verificados por comando;
  reconciliar F8.
- S6 (NOV-78 Paso 57): cierre trazable obligatorio en CI + revision
  independiente.
- S7 (NOV-79 Paso 58): benchmarks automatizados + claim de cache
  respaldado.
- S8 (NOV-80 Paso 59): segundo ciclo real de auto-mejora con tokens
  reales y corpus ampliado.
- S9 (NOV-75 Paso 60): higiene documental y evidencia versionada.

## Tasks

| ID | Spec | Route | Estado |
|----|------|-------|--------|
| T1 | S1 | NOV-72 labels + conformidad + release 0.15 | in_progress |
| T2 | S2 | NOV-73 tokens e2e | pending |
| T3 | S3 | NOV-76 journeys 2 hosts | pending |
| T4 | S4 | NOV-77 corpus ampliado | pending |
| T5 | S5 | NOV-74 Done + reconciliacion | pending |
| T6 | S6 | NOV-78 cierre obligatorio | pending |
| T7 | S7 | NOV-79 benchmarks | pending |
| T8 | S8 | NOV-80 segundo ciclo | pending |
| T9 | S9 | NOV-75 higiene docs | pending |

## Log

- L1 (usuario, verbatim): "excelente, ahora volvamos a develop para continuar con algunas mejoras que tenemos en linear. analiza las mejoras de la nueva fase que se agregó e intentemos realizarlas sin afectar las evaluaciones de CI"
- L2 (usuario, verbatim): "hacer toda la fase en orden como lo explica linear, luego al finalizar cada tarea debemos actualizar la tareas en linear de la misma forma que estan las tareas finalizadas con la misma informacion como evidencia"
- L3: F9 = milestone d757bf0f (0%); F1-F8 100%. Issues: NOV-72/73/76/77/74/78/79/80/75 (Pasos 52-60). Convencion de cierre: docs/close-traceability.md (comentario evidencia + task close --issue; P0 con veredicto independiente). Full bodies de Linear truncados en MCP (sin get_issue): se trabaja con objetivos + estado del repo; pedir cuerpo completo si un paso lo exige.
- L4 (T1 NOV-72): labels ubuntu-24-latest -> ubuntu-24.04 en release.yml x2 y fuzz.yml x1 (docs GitHub confirman que el label no existe); YAML validado con js-yaml; sin tests que lean esos archivos. Pendiente (cuerpo truncado en MCP + requiere autorizacion): evidencia de hosts en conformidad, bump a 0.15 (hoy tag maximo v0.13.0, versioninfo 0.13.0) y release de prueba con tag real (= publicar).
- L5 (T1 NOV-72 parcial, commit f5cee6b): labels, lint allowlist+actionlint, host-conformity (opencode@1.19.5 + HOST_API_KEY opcional), version 0.15.0 + CHANGELOG F8, deriva verde local. Pendiente CI: push (usuario), tag v0.15.0-rc1 de prueba (= publicar, requiere autorizacion explicita), workflow_dispatch de fuzz, evidencia para Linear.
- L6: usuario decide push-only (tag v0.15.0-rc1 despues de ver CI). Pendiente: push develop (usuario), CI verde, dispatch fuzz, tag de prueba, evidencia Linear NOV-72.
- L4 (T2 NOV-73 en curso): verificados formatos reales en esta maquina — opencode session_v2 (tokens input/output/reasoning/cache, 303 sesiones), claude transcripts message.usage, codex rollout token_usage_record. SDK del plugin: execute.after NO expone usage (tool/session/agent/message/input). Implementado internal/trace/hostreaders.go (3 parsers read-only + fixtures anonimizadas/malformed/hostiles, 6 tests verdes, CRAP <=12). Pendiente: hook TS defensivo + flags run/execution + medicion real + doc no-H10.
- L5 (T2 NOV-73 parcial, sin Linear aun): readers opencode/claude/codex + fixtures + 6+3 tests; flag --execution-id + round-trip; hook usageFlags defensivo + 6 tests bun (184 lib OK); E2E real opencode (ses_e1f2e68, 4307 in +33 out measured-host); doc vivo docs/host-usage.md, seccion H10 mudada con puntero; deriva verde; suite cli verde. Pendiente: CI (push usuario), tag, fuzz, evidencia Linear.
- L6: Linear actualizado (sin push): comentarios de evidencia T1 NOV-72 y T2 NOV-73 posteados con formato de finalizadas (que/resultados/comandos/commits); estado NO pasado a Done (pendiente CI + tag + fuzz + P0 veredicto independiente). Cuando CI valide, se completa la evidencia y se pide el Done.
