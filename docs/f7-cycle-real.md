# Primer ciclo real de auto-mejora (NOV-71 / T47) — 2026-11-14

Hipótesis: recortar los prompts de `agent-tasks-v1` (~81→~40 chars, -50%)
mantiene el pass-rate. Superficie reversible y no crítica (textos de
prompt; checks, splits y harness intactos). Canary: no aplica
promoción parcial — la decisión es adoptar o mantener.

## Protocolo

- Suite `agent-tasks-v1`, splits `agent-splits-v1` (4 exploración + 1 holdout).
- Host: `opencode run --format json --model opencode-go/beam-free`
  (único host ejecutable sin créditos; claude sin login, codex sin
  binario, devin sin CLI, opencode Models sin saldo).
- 1 repetición por semilla fijada (seed 20261114); variabilidad entre
  corridas registrada tal cual; n < 10 → sin afirmación de
  significancia (f7-protocol-v1).
- Tokens de host: `unknown` (el host no expone conteos en este modo);
  costo medido = latencia + proxy local; nunca se suma unknown como cero.

## Baseline (código vigente, prompts originales, seed 20261114)

`bench agent-tasks --reps 1 --split all --model opencode-go/beam-free
--budget-s 550 --variant baseline --save f7-cycle-base.json`

| Task | Split | Resultado | Detalle |
|------|-------|-----------|---------|
| AT1 | exploración | pass | go test ok |
| AT2 | exploración | pass | go test ok |
| AT3 | exploración | pass | node ok |
| AT4 | exploración | pass | diff limpio |
| AT5 | holdout | pass | go test ok (confirmación no expuesta al candidato) |

5/5 evaluable, pass-rate 1.00, 0 timeout, 0 infra. Evidencia:
`f7-cycle-base.json` (5 outcomes con seed, latencia y fuente).

## Candidato (prompts recortados ~81→~40 chars, -50%)

Misma suite, mismos seeds. Resultado: 5/5 pass, pass-rate 1.00.
Evidencia: `f7-cycle-cand.json`.

Comparación robusta (`bench agent-compare --base f7-cycle-base.json
--candidate f7-cycle-cand.json`):

- veredicto: **TIE_COST_TIEBREAK** — igual métrica primaria, baseline kept, costo empatado (unknown en ambas ramas).
- baseline n=5 (evaluable 5), candidate n=5 (evaluable 5); gap 0.0000 descriptivo, sin significancia (n<10).

## Control negativo (variante saboteada: AT3 "no modifiques nada")

Evidencia: `f7-cycle-neg.json` — AT3 fail (test.mjs sin tocar),
resto 3 pass. `agent-compare` baseline vs sabotaje:

- veredicto: **KEEP_BASELINE** — "candidate regresses the primary
  metric: non-regression rule keeps the baseline" (gap -0.25
  descriptivo, n=5/4).
- El evaluador rechaza de verdad: la primera reversión real del ciclo.

## Decisión

**MANTENER baseline** por veredicto (TIE no adopta con MinDelta 0.0
estricto). Humano consultado: eligió **ADOPTAR recortados** con motivo
registrado (igual calidad 5/5 + 4/4 post-adopción, prompts -50% para
futuras corridas; el guard anti-gaming se reevalúa con n mayor y
tareas más duras). Aprobación humana registrada: decisión explícita
del operador, contra veredicto TIE y recomendación de mantener.

Seguimiento post-adopción (`adopted-trimmed`, mismos seeds): 4/4
evaluable pass (AT5 holdout timeout del tier gratuito, no-evaluable),
sin regresiones.

Costo total del experimento: ~20 corridas de agente (~1-3 min c/u
en tier gratuito), 0 tokens medidos del host (unknown etiquetado),
proxy local intacto.

Lecciones (admitidas como candidatas, no auto-adoptadas):
- El tier gratuito sirve para el ciclo pero mete timeouts aleatorios
en corridas largas; el protocolo los bucketiza como no-evaluables
en vez de fallos (visto en corridas previas AT1/AT4, no en la final).
- Los checks de Go compilan binarios que App Control a veces bloquea;
  el runner los clasifica como infra (NOV-68), no como fail.
- Prompts más cortos no degradan en esta suite (n=5): señal para
  repetir con n mayor antes de afirmar ahorro.
