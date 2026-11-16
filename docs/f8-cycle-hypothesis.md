# Segundo ciclo de auto-mejora — hipótesis pre-registrada (NOV-80 Paso 59)

Estado: DECLARADA, no ejecutada (2026-11-16). La ejecución requiere
ventana con créditos y binarios ejecutables (ver § Bloqueadores).
Nada aquí afirma resultados: los veredictos salen de `agent-compare`
sobre evidencia versionada, nunca de este documento.

## Hipótesis

Si se fuerza `TE_TOOLSET=minimal` (vs baseline `auto`, que en este
repo resuelve `full` por diseño con `charts/` presente) en
`agent-tasks-v3 --reps 3`, entonces el `pass-rate` de exploración cae
a lo sumo `MinDelta 0.0` (criterio estricto heredado del ciclo 1) y la
suma de tokens `measured-host` del candidato es menor que la del
baseline, con el % `unknown` reportado aparte (nunca sumado como cero).

Superficie: solo disciplina + guidance del toolset
(`opencode/lib/domain-toolset.ts`, `opencode/plugins/regulus.ts`).
Reversible por env (`TE_TOOLSET=full`, fail-closed a full); no toca
checks, splits ni harness. Distinta de la del ciclo 1 (recorte de
prompts, `docs/f7-cycle-real.md`).

## Métricas

- Primaria (calidad): `pass-rate = Pass / Evaluable` solo con outcomes
  `pass/fail`; `timeout/infra/not-run` son buckets no-evaluables que
  nunca pasan (`internal/bench/agenttasks.go`, `SummarizeAgent`).
- Costo (Paso 53): `Σ tokens_in + tokens_out` solo con
  `token_source == measured-host` (lectores `hostreaders`,
  `docs/host-usage.md`); `unknown` se reporta, no se suma.
- `n = 44 tareas × 3 reps = 132 intentos` (102 exploración + 30
  holdout, splits v3 seed 20261114). Si los evaluables no superan
  `RobustSignificanceFloor`, el resultado es `INCONCLUSIVE`, no una
  afirmación.

## Decisión pre-registrada

Solo `ADOPT` adopta. `TIE_COST_TIEBREAK` mantiene baseline (precedente
del ciclo 1: `docs/f7-cycle-real.md`, humano adoptó contra veredicto
con motivo registrado — esta vez la decisión humana también queda
registrada si ocurre).

## Controles

- Negativo: variante saboteada (`AT3 no modifiques nada`) + trampas
  `AT28/AT29` (hardcode pasa lo visible, falla lo oculto) → se espera
  `KEEP_BASELINE`.
- Positivo/calibrador: A/A baseline vs baseline en humo
  (`--task AT1 --reps 3`) → se espera `TIE`; calibra el ruido del
  tier gratuito y sus timeouts aleatorios.

## Comandos (cuando se ejecute)

```bash
bin/regulus bench agent-tasks --reps 3 --split exploration \
  --model opencode-go/beam-free --host opencode \
  --evidence-dir bench/evidence/v3-cycle2-exploration-beamfree
bin/regulus bench agent-tasks --reps 3 --split holdout \
  --model opencode-go/beam-free --host opencode \
  --evidence-dir bench/evidence/v3-cycle2-holdout-beamfree
bin/regulus bench agent-report --evidence-dir bench/evidence/v3-cycle2-exploration-beamfree
bin/regulus bench agent-compare --base <base-manifest> --candidate <cand-manifest>
```

## Bloqueadores de ejecución (2026-11-16)

- App Control bloquea binarios frescos (checks Go → `infra`); solo
  corren binarios con reputación (`bin/regulus.exe` firmado actual).
- Tier gratuito: timeouts aleatorios → no-evaluables; `402/quota` →
  corrida `NOT RUN` (reintentar en otra ventana, nunca relajar umbrales).
- Multi-host/modelo (claude/codex/devin, segundo tier de pago):
  pendiente de créditos y logins (ver NOV-69/NOV-76).
