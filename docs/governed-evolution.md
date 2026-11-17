# Gobernanza de Self-Evolution (F12.7 / NOV-102)

Cierra el ciclo de mejora: detectar debilidad, formular hipótesis,
generar candidatos y promover solo con evidencia y gobernanza. La
decisión se prueba con el protocolo F11.6; esta capa la gobierna:
quién construye, quién puntúa, quién aprueba, quién despliega y bajo
qué condiciones se revierte.

## De dónde sale cada pieza (sin duplicar)

- **Señales**: F12.4 (`quality`: baselines, detecciones, alertas) como
  entrada; evals (manifiestos F11.3, ventanas F12.4) y memoria
  verificada F12.6 (`learn`) como hipótesis. El ciclo guarda punteros
  (`source`, `ref`), nunca cuerpos: la evidencia vive en su store.
- **Comparación**: el plan F11.6 enlazado decide (matriz declarada
  seeds × reps × modelos, veredicto de exploración congelado, holdout
  sellado que confirma sin re-decidir). Aquí no hay una segunda regla.
- **Canary/rollback**: el camino gobernado existente (`promote.go`:
  scope limitado prompt/skill/workflow reversible no-crítico,
  aprobación humana, `GovernedRollback`). El ciclo lo invoca con
  condiciones preaprobadas, no lo reimplementa.
- **Coste**: F11.5 (tokens medidos se suman, `unknown` se cuenta jamás
  se suma). **Seguridad**: marcadores de inyección + dominio.
  **Permisos/políticas**: F11.8 (aprobaciones ligadas a hashes).
- **Regresiones**: la regla de no-regresión (gap < 0 o MinDelta no
  alcanzado = baseline) bloquea la promoción.

## El ciclo

```
DECLARED -> CANDIDATES -> COMPARED -> REVIEWED -> APPROVED -> CANARY -> PROMOTED
                                                                    -> ROLLED_BACK (desde CANARY o PROMOTED)
cualquier punto previo -> BLOCKED (terminal: evidencia insuficiente,
regresión, review rechazada, holdout contradictorio)
```

Un ciclo bloqueado nunca promueve: una pregunta nueva necesita un
ciclo nuevo. Todo queda en `.regulus/evolution/governed/gov_<id>.json`
con pin SHA de la declaración (signals, hipótesis, superficie,
roles, sensibilidad, K); la deriva falla en vez de compararse.

## Declarar

```bash
cat > /tmp/cycle.json <<'EOF'
{
  "signals": [
    {"source": "quality", "ref": "quality/baseline/v3", "summary": "p90 drift on retry storms"},
    {"source": "memory", "ref": "learn:retry-cap-pattern", "summary": "verified cap-at-2 pattern"}
  ],
  "Hypothesis": {
    "Problem": "latency spikes", "RootCause": "unbounded retries", "Change": "cap at 2",
    "Benefit": "lower p90", "PotentialRegressions": "flaky fails", "EvaluationCriteria": "delta >= 0"
  },
  "surface": {
    "kind": "workflow", "description": "retry policy", "reversible": true,
    "revert_command": "regulus evolve rollback --exp <id> --reason <t>"
  },
  "roles": {"proposer": "sextus", "evaluator": "andragon", "approver": "ada", "deployer": "ops"},
  "sensitivity": {"touches_policy": true},
  "max_candidates": 2
}
EOF
regulus evolve validation cycle-declare --spec /tmp/cycle.json
# verdict: GOVERNED_DECLARED gov_<id> (sha <12>, K=2, sensitive=true)
```

Reglas de declaración:

- ≥1 señal (`quality|eval|memory|bench|trace`, con `ref` al store).
- Hipótesis completa (los 6 campos) y superficie **reversible**
  obligatoria (`revert_command` presente).
- **Roles separados**: los cuatro distintos entre sí (nadie aprueba
  su propio cambio crítico por construcción) y approver humano
  (nunca auto/bot).
- K acotado: `max_candidates` 1..5. Más ideas = ciclo nuevo.

## Candidatos, plan y comparación

```bash
regulus evolve validation cycle-propose --cycle gov_<id> --candidate retry-cap-2 \
  --content "cap retries at 2 with backoff" --by sextus
# solo el proposer construye (el evaluador nunca implementa);
# el contenido con inyección se rechaza en la puerta, con redacción previa al store
regulus evolve validation cycle-link --cycle gov_<id> --plan val_<id>
# el plan debe verificar y su evaluator/implementer deben ser el evaluator/proposer
# del ciclo: independencia en ambas capas
regulus evolve validation cycle-compare --cycle gov_<id> --by andragon
# pliega las 5 dimensiones: quality, latency, cost, security, regressions + holdout
```

`compare` congela el veredicto del plan y lo evalúa:

- `ADOPT_CANDIDATE` → `COMPARED` (sigue).
- `TIE_COST_TIEBREAK` con candidato estrictamente más barato también
  sigue (el coste decide empates, nunca promueve solo).
- `KEEP_BASELINE` / `INSUFFICIENT_EVIDENCE` → **BLOCKED** con su
  evidencia (exit 1).
- Holdout contradictorio sobre un `ADOPT` → **BLOCKED**: el holdout
  confirma, nunca re-decide; un `COMPARED` nunca se re-compara.

## Revisión, aprobación, canary, promoción, rollback

```bash
regulus evolve validation cycle-review --cycle gov_<id> --reviewer mira \
  --verdict pass --reason "probed sabotage and markers: held"
# reviewer fuera de los 4 roles (independiente de build, score, approval y deploy);
# reject bloquea terminalmente
regulus evolve validation cycle-approve --cycle gov_<id> --by ada \
  --reason "risk accepted" --commit abc1234 --policy mcp-policy/v7
# solo el approver fijado; cambios sensibles (Core/permisos/políticas/evaluadores
# o superficie crítica) exigen --commit y --policy: la aprobación ata bytes exactos
# a una política exacta
regulus evolve validation cycle-canary --cycle gov_<id> --by ops --kind workflow \
  --conditions "p90 up 20%,pass-rate down" --reason "reversible surface"
# scope prompt|skill|workflow reversible no-crítico + ≥1 condición de rollback preaprobada
regulus evolve validation cycle-monitor --cycle gov_<id> --by ops --healthy true --detail "..."
regulus evolve validation cycle-promote --cycle gov_<id> --by ops --winner retry-cap-2
# solo desde canary totalmente sano (≥1 observación, 0 unhealthy)
regulus evolve validation cycle-rollback --cycle gov_<id> --by ops \
  --reason "p90 up 35%" --condition "p90 up 20%"
# solo sobre condición preaprobada; supersedea al ganador, la evidencia queda
regulus evolve validation cycle-report --cycle gov_<id>
# solo estados terminales: informe reproducible con todo lo anterior + veredicto y next
```

## Lo que este protocolo no hace

No ejecuta benchmarks (F11.2/F11.3), no mide tokens (F11.5), no traza
(F11.1), no versiona conocimiento (F12.6). Los ingiere y los cita.
Done no demuestra los criterios: solo un informe terminal con su pin,
sus N, su review independiente, su aprobación con hashes y su canary
monitoreado los demuestra.
