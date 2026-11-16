# Validación reproducible de Evolution (F11.6 / NOV-92)

Protocolo que demuestra que un cambio de Evolution mejora calidad,
latencia o coste con evaluación independiente y rollback. La hipótesis,
la superficie reversible, el baseline, las métricas y el presupuesto se
declaran ANTES de experimentar y se congelan bajo un pin SHA-256: la
deriva posterior falla la verificación en vez de compararse en silencio.

## Declarar la hipótesis pre-experimento

Todo empieza por `declare` con un spec JSON. Nada corre antes: el plan
nace `DECLARED` y el primer trial lo mueve a `RUNNING`.

```bash
cat > /tmp/val-plan.json <<'EOF'
{
  "Hypothesis": {
    "Problem": "latency spikes on retry storms",
    "RootCause": "unbounded retries",
    "Change": "cap retries at 2 with backoff",
    "Benefit": "lower p90 without pass-rate loss",
    "PotentialRegressions": "flaky tasks may fail instead of retry-passing",
    "EvaluationCriteria": "pass-rate delta >= 0 and p90 down on exploration"
  },
  "surface": {
    "kind": "workflow",
    "description": "retry policy only",
    "reversible": true,
    "revert_command": "regulus evolve rollback --exp <id> --reason <text>"
  },
  "baseline": "retry-baseline-v1",
  "candidate": "retry-cap-2",
  "dataset": "agent-tasks-v3",
  "tier": "L2",
  "models": ["model-a", "model-b"],
  "seeds": [20261116, 20261117],
  "repetitions": 2,
  "primary": "pass-rate",
  "min_delta": 0.0,
  "budget": {"max_runs": 32, "max_wall_time_ms": 60000, "max_tokens": 100000},
  "implementer": "sextus",
  "evaluator": "andragon",
  "evaluator_version": "robust-compare-v1",
  "commands": ["bin/regulus bench agent-tasks --reps 2 --split exploration"]
}
EOF
regulus evolve validation declare --plan /tmp/val-plan.json
# verdict: VALIDATION_DECLARED val_<id> (sha <12>)
```

Reglas de la declaración (todo rechazo sale exit 1 con el motivo):

- Hipótesis completa (los 6 campos) y con soporte generalizable: un
  caso aislado nunca es regla (precedente `CheckSupport`, S2).
- Superficie reversible obligatoria (`reversible: true` + `kind`,
  `description` y `revert_command`): sin rollback probable no hay
  protocolo. `critical: true` se admite pero queda marcado: la
  aprobación humana de `finalize` asume ese riesgo.
- Ejecución factible: modelos comparables no vacíos y sin duplicados,
  seeds no-cero sin duplicados, `repetitions >= 1`, `min_delta >= 0`,
  tier en el vocabulario F11.3 (`L0..L3`), y `budget.max_runs` que cubre
  la matriz `seeds × reps × models`. `max_wall_time_ms` y
  `max_tokens` se reportan contra el presupuesto, no se elevan aquí.
- Evaluador independiente: `evaluator` no vacío, con versión, y
  distinto del `implementer` (auto-evaluación = rechazo).
- Al menos un comando exacto. Texto libre (baseline, candidate,
  notes, commands) con redacción previa de secretos, igual que el
  manifest F7.

## Ejecutar candidatos (matriz declarada)

```bash
regulus evolve validation run --plan val_<id> --arm baseline --task AT1 \
  --seed 20261116 --rep 0 --model model-a --status pass \
  --latency-ms 120 --tokens 40 --source measured-host \
  --receipt rc_AT1 --trace run_AT1
```

Cada trial cita su `arm` (baseline|candidate), `seed`, `rep`, `model`
(dentro de la matriz o rechazo), `status` F11.3
(pass|fail|timeout|infra|non-evaluable), latencia, tokens con su fuente
F11.5 y los punteros F11.1 (`receipt`, `trace`). El presupuesto
`max_runs` es techo duro; el resto es adherencia reportada.

## Comparar con holdout gobernado

```bash
regulus evolve validation compare --plan val_<id>
```

`compare` pliega solo la partición `exploration` y congela el veredicto
(`ADOPT_CANDIDATE` | `KEEP_BASELINE` | `TIE_COST_TIEBREAK` |
`INSUFFICIENT_EVIDENCE`, mismo vocabulario que `CompareRobust`):

- Calidad, latencia y tokens por separado, cada una con N y
  variabilidad (pass-rate sobre evaluables; latencia media/min/max;
  tokens medidos sumados y `unknown` contado, jamás sumado).
- `infra` nunca diluye la calidad; sin evaluables en un brazo el
  veredicto es `INSUFFICIENT_EVIDENCE` (exit 1: no se finaliza).
- Debajo del suelo de significancia (`n < 10`) no se afirma
  significancia; por encima tampoco: el gap es descriptivo.
- Si los brazos corrieron modelos distintos, queda como salvedad
  explícita (comparabilidad limitada), no como silencio.

El holdout abre solo después, solo en tier L3, solo con auditoría:

```bash
regulus evolve validation holdout-run --plan val_<id> --arm candidate ... --audit "audit: L3 pre-release gate"
```

El holdout confirma (acuerdo/desacuerdo descriptivo), nunca re-decide:
si contradice un `ADOPT`, `finalize` lo rechaza y pide un plan nuevo.
Tier L3 sin trials de holdout no finaliza (el tier promete full+holdout).

## Descarte, controles y evaluador

```bash
regulus evolve validation control --plan val_<id> --kind negative --name sabotage \
  --expectation KEEP_BASELINE --outcome KEEP_BASELINE --held true --detail "..."
regulus evolve validation discard --plan val_<id> --candidate retry-cap-3 \
  --reason "infra noise, not signal" --by andragon
regulus evolve validation link --plan val_<id> --exp exp_<id>
```

- Controles `positive` (A/A: el ruido del tier queda calibrado) y
  `negative` (sabotaje: el evaluador debe rechazar de verdad).
- El informe exige al menos un rechazo real: un descarte registrado o
  un experimento enlazado cuyo estado vivo sea `REJECTED` o
  `ROLLED_BACK` (se carga en `finalize`, no se confía en el enlace).
- `finalize` exige aprobador humano (`--approved-by`, nunca auto/bot),
  motivo, y la decisión explícita sobre cambios heredados
  (`--inherited adopt|keep|revisit` + `--inherited-reason`): nada de
  herencias por defecto.

## Decidir e informar

```bash
regulus evolve validation finalize --plan val_<id> --verdict TIE_COST_TIEBREAK \
  --approved-by andragon --reason "tie keeps baseline; no regression adopted" \
  --inherited keep --inherited-reason "NOV-71 trims stay; nothing else touched" \
  --commits abc1234 --manifests bench/evidence/v/manifest.json
regulus evolve validation report --plan val_<id>
```

`finalize` comprueba, en orden: integridad del pin, estado `COMPARED`,
que no entraron trials nuevos tras el veredicto, igualdad
veredicto-congelado, aprobación humana, herencia explícita, pins de
commits y manifests, prueba de rechazo, acuerdo del holdout y promesa
L3. La promoción y el rollback siguen por el camino gobernado
existente (`adopt` con aprobación, canary, `GovernedRollback`); esta
capa prueba la decisión, no la aplica.

El informe reproducible trae: declaración + sha, hipótesis, superficie,
matriz, evaluador vs implementador, comandos exactos, las tres
dimensiones con N y variabilidad, confirmación de holdout con sus
auditorías, controles +/-, descartes y rechazos enlazados con estado
vivo, aprobación humana, decisión de herencia, commits, manifests,
receipts y traces, veredicto y next step. Sin regresiones adoptadas y
sin significancia afirmada.

## Reconciliación NOV-80 / NOV-71

Qué evidencia se reutiliza y qué faltaba:

- NOV-71 (`docs/f7-cycle-real.md`, ciclo 1 real): hipótesis de recorte
  de prompts, 5/5 TIE con `agent-compare`, **control negativo real**
  (sabotaje AT3 → `KEEP_BASELINE`, primera reversión real) y adopción
  humana contra veredicto con motivo registrado. Esa es la prueba de
  rechazo que este protocolo exige citar: el informe F11.6 la enlaza
  como experimento `REJECTED` (o la registra como descarte) en vez de
  recontarla de palabra.
- NOV-80 (`docs/f8-cycle-hypothesis.md`, ciclo 2): hipótesis
  pre-registrada `TE_TOOLSET=minimal` (n=132, MinDelta 0.0, controles
  +/- declarados, suelo de significancia). Estado: DECLARADA, no
  ejecutada (bloqueadores: App Control sobre binarios frescos, tier
  gratuito con timeouts, ventana de créditos pendiente).
- Qué faltaba y ahora se cubre: el pin SHA que congela la
  pre-registración (antes el doc podía derivar sin que nadie lo
  notara); la matriz seeds/reps/modelos exigible (antes intención);
  el evaluador independiente verificable (antes convención); el
  holdout sellado con puerta auditada (antes promesa); el registro de
  descartes (antes memoria); la decisión de herencia explícita (antes
  implícita); y el informe separable por dimensión con N y
  variabilidad (antes veredicto + narrativa).

El primer informe F11.6 completo sale de ejecutar NOV-80 bajo este
protocolo cuando haya ventana con créditos; hasta entonces, los tests
(`internal/evolve/validation_test.go`,
`internal/cli/evolve_validation_test.go`) ejercitan el ciclo entero
con matrices deterministas, incluido un rechazo real enlazado y un
rollback citado.

## Lo que este protocolo no hace

No ejecuta benchmarks (eso es F11.2/F11.3), no mide tokens (eso es
F11.5 con sus readers), no traza ejecuciones (eso es F11.1), no adopta
ni revierte por sí mismo (eso es el ciclo gobernado + canary/rollback
de `promote.go`). Los consume y los cita. Done no demuestra los
criterios: solo un informe `FINAL` con su pin, sus N y su rechazo real
los demuestra.
