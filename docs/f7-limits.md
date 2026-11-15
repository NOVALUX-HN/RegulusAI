# F7 — Límites declarados (NOV-61)

Versión: `f7-protocol-v1` · Fecha: 2026-11-14 · Estado: normativo para
NOV-59/60/61. Complementa `docs/f7-protocol.md` sin redefinirlo.

## 1. Alcance

- F7 explora mejoras candidatas y las compara contra la línea base con el
  evaluador robusto (`internal/evolve/robust.go`, `robust-compare-v1`).
  La exploración nunca adopta; la promoción es gobernada con canary
  (`internal/evolve/promote.go`).
- No redefine: NOV-35 (bench `macizo-bench-v3`, B1–B14), NOV-39 (tokens y
  costo), NOV-41 (corpus adversarial), NOV-42 (ciclo baseline/evaluate/
  adopt/reject/rollback), NOV-34 (`run_id`/`execution_id`), ni F6
  (`docs/contract-neutral.md` y matriz de conformidad).

## 2. Evaluador robusto (NOV-59)

- Primaria: `pass_rate = pass / (pass + fail)`. `timeout`/`infra`/
  `non-evaluable`/`missing` no puntúan y nunca se premian.
- Guardrails duros (funcional, cumplimiento, no-regresión, adversarial,
  mantenibilidad): cualquier fallo ⇒ `REJECT_GUARDRAIL`, aunque la
  primaria mejore.
- No-regresión + mejora mínima: regresión ⇒ `KEEP_BASELINE`; mejora
  menor que `MinDelta` ⇒ `KEEP_BASELINE`; empate ⇒ `TIE` con la base.
  Por defecto `MinDelta = 0.0` (mejora estricta); subirlo es decisión
  humana versionada, el candidato nunca lo toca.
- Costo/latencia/rework solo desempatan (`candidate-cheaper`,
  `baseline-cheaper`, `equal`); un ahorro con regresión es REJECT.
- Composición (`ComposeLineage`) crea linaje, no puntaje: el compuesto
  debe ejecutarse y evaluarse con evidencia propia.
- Holdout (B11–B14) solo confirmación final auditada
  (`ConfirmHoldoutWithAudit(audited=true, auditRef=…)`). Sin auditoría se
  rechaza. La búsqueda nunca ejecuta IDs del holdout; nombrarlos en una
  propuesta es leakage y se rechaza.
- Reporte honesto: `n`, dispersión (`pass/fail/infra/rate` por lado) e
  incertidumbre. Bajo el piso (`n < 10`) no se afirma significancia;
  sobre el piso tampoco se testea significancia: la brecha es
  descriptiva.
- Evaluador y umbrales inmutables por candidato
  (`VerifyEvaluatorUntouched`, `VerifyRobustThresholdsUntouched`).

## 3. Promoción gobernada (NOV-60)

- Estados: se añaden `CANARY` y `ROLLED_BACK`; `PROPOSED/RUNNING/
  EVALUATED/ADOPTED/REJECTED` y sus bordes existentes no cambian.
  `approved` es artefacto (evento `approval` en historial), no estado:
  `EVALUATED → [approval] → CANARY → ADOPTED`, con `CANARY/ADOPTED →
  ROLLED_BACK` como recuperación.
- Canary limitado: `kind ∈ {prompt, skill, workflow}`, reversible y no
  crítico. Fuera de eso se rechaza.
- Aprobación humana obligatoria (`By` + `Reason`, sin auto-aprobación)
  para entrar a canary y para adoptar. Contenido de alto impacto
  (permisos, gates, seguridad, límites, cuotas, producción, rollout y
  fundamentales INV) siempre exige humano; `RequiresHumanApproval` lo
  decide de forma pura.
- Adopción desde canary reutiliza las garantías del ciclo: dominio,
  `TrustCheck.Verify`, baseline almacenada y `Adoptable`. Sin snapshot o
  sin humano se rechaza.
- Rollback atómico (`GovernedRollback`): valida `by/reason/manifest/
  diff/receipts(≥1)` antes de escribir; supersede de versión + pase a
  `ROLLED_BACK` + artefacto `versions/rollback-N.json` (temp+rename). Si
  un paso tardío falla, el artefacto parcial se retira.
- Post-adopción (`PostAdoptionCheck`) re-chequea contra la baseline con
  `Adoptable`; una regresión posterior dispara rollback gobernado.
- Sin auto commit/push/release: la promoción no ejecuta git ni shell;
  solo escribe bajo `.regulus/evolution/` y el workspace aislado.
- Resume idempotente: `EnterCanary` en `CANARY` es no-op;
  `ResumeCanary` re-lee `EVALUATED/CANARY/ADOPTED/ROLLED_BACK` sin
  re-disparar probes.

## 4. Cobertura E2E + adversarial (NOV-61)

- Journey (`internal/evolve/f7_e2e_test.go`): baseline → propuesta →
  candidatos → ejecución → evaluación → decisión → aprobación → canary →
  seguimiento/rollback, en paralelo y secuencial, `k=1` y `k>1`, seeds
  fijas, holdout auditado, parciales que no abortan, cancel/timeout que
  sí abortan, resume idempotente y presupuestos (`MaxCandidates`,
  `MaxTokens`, `MaxDiffBytes`, `MaxConcurrency`, `MaxRetries`).
- Adversarial: leakage de rúbricas/holdout, manipulación de resultados
  (hash de manifest), prompt injection, debilitamiento de gates (brecha
  DOM), self-grading (vacío ⇒ `INSUFFICIENT`), manifest corrupto, drift
  de split, colusión (linaje sin veredicto), bypass de aprobación y
  umbrales inmutables. Todo con asserts deterministas.
- Multi-runtime: el experimento declara `real:` o `degraded:` en
  `Runtime`. Un `degraded` nunca afirma paridad con `real`.
- Recursión conservadora por defecto: `K=1` un paso, `MaxRetries=0`
  (infra no reintenta salvo presupuesto explícito), encadenamiento por
  `parent_id` fijado por el llamante.

## 5. Lo que F7 no promete

- Sin significancia estadística: F7 describe brechas, no las certifica.
- Sin ejecución del holdout durante la búsqueda; sin adopción por costo;
  sin canary para superficie crítica o irreversible; sin promoción sin
  humano; sin mutación de evaluador/umbrales por el candidato.
- Evidencia por referencia (manifest + SHA-256): el drift falla en
  `Verify`, no se compara contra contenido movido.

## 6. Lo que mostró el primer ciclo real (NOV-71, 2026-11-14)

Ver `docs/f7-cycle-real.md` (baseline 5/5, candidato trimmed 5/5 TIE,
control saboteado rechazado). Los tests sintéticos no cubrían:

- Timeouts del tier gratuito en corridas largas (el agente real a
  veces supera el budget): el protocolo ya los bucketiza como
  no-evaluables, pero el reporte debe decir cuántos hubo por corrida.
- Bloqueo de App Control sobre binarios de check compilados en Temp:
  el runner los clasifica como infra (NOV-68); en este ciclo no hubo
  ninguno en la corrida final, pero sí en corridas previas.
- El agente real escribe fuera del fixture si el prompt no ancla el
  directorio de trabajo: `runOne` antepone el workdir al prompt.
- Tokens de host `unknown` en modo headless JSON de opencode: el
  ciclo decide sin ellos (costo = latencia + proxy), etiquetados.
- n=5 no da margen medible con baseline al 100%: el próximo ciclo
  necesita tareas más duras o n mayor para que TIE no sea el techo.
