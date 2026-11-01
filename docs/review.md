# Proporcional review — `regulus review` (F2)

El gauntlet completo corre siempre (~5 min). El review proporcional basta
en riesgo medio: congela el candidato, clasifica el riesgo con la escala
existente (`task classify`) y aplica **una sola lente**, en segundos.

## Uso

```bash
regulus review [--task <t>] [--lens reviewer|crap|mutation|spec|evidence|gate]
```

Ejemplo en este repo (tras tocar código):

```bash
regulus review --task macizo-flow/F2
# review: frozen head=1a2b3c4d 3 file(s) (candidate snapshot; read-only, nothing committed)
# review: risk=medium class=STANDARD lens=reviewer (code change — senior read of touched files)
# lens reviewer: senior read of touched files (not just the diff)
#   internal/cli/review.go: 412 line(s), 14880 B
#   ...
# review: verdict INFORMATIONAL — nothing blocked, nothing committed, no gates changed
# review: full proof stays `regulus gate run <task>`; this review is opt-in and advisory
```

Forzar una lente (opt-in explícito):

```bash
regulus review --lens spec --task macizo-flow/F2
```

## Congelado (freeze)

Antes de leer nada, el review congela el candidato como RDD: `HEAD` más
el estado del worktree (`git status --porcelain -z`). La cabecera
`frozen` lo cita, así el lector humano sabe sobre qué snapshot opina.
Si git no responde, el riesgo escala a `high` (UnclearRisk) en vez de
fallar: la incertidumbre nunca se disfraza de limpieza.

## Riesgo → lente (una sola)

El riesgo reusa `sextant.Classify` (las mismas señales que
`task classify`): código, destrucción, seguridad, contratos,
concurrencia, delivery. La clase manda el riesgo:

| Clase    | Riesgo   | Qué pasa                                    |
|----------|----------|---------------------------------------------|
| TRIVIAL  | low      | 1 lente rápida; nada que probar             |
| STANDARD | medium   | 1 lente en segundos (el caso proporcional)  |
| COMPLEX  | elevated | 1 lente + se recomienda el gauntlet         |
| CRITICAL | high     | 1 lente informativa + el gauntlet decide     |

La lente automática mira qué toca el cambio:

| Cambio toca              | Lente      | Qué hace (segundos, sin ejecutar el gate) |
|--------------------------|------------|--------------------------------------------|
| solo docs                | `spec`     | `spec.Check` sobre los charts tocados      |
| solo config de gates/CI  | `gate`     | lista el gauntlet que el cambio enfrentaría|
| solo recibos             | `evidence` | cuenta recibos por veredicto (`--task`)    |
| tests dominan (Go)       | `mutation` | enuncia la política + último recibo        |
| Go con cobertura a mano  | `crap`     | CRAP de funciones tocadas con el perfil    |
| resto / árbol limpio     | `reviewer` | lectura senior: tamaños, marcadores, test  |

`--lens` fuerza cualquiera de las 6, siempre informativa.

## Opt-in y sin debilitar

- **Informativo**: exit 0 aunque haya hallazgos; `INFORMATIONAL` en el
  parte. Nunca bloquea.
- **Solo lectura**: no commitea, no escribe recibos, no toca gates ni
  charts. La entrega es el humano leyendo stdout.
- **Sin debilitar**: `regulus.json` y los 12 gates no cambian; la prueba
  completa sigue siendo `regulus gate run [--profile full] <task>`.
  Alto/CRITICAL siempre van al gauntlet.
