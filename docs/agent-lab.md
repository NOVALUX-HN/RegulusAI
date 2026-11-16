# Agent Lab — ejecución aislada y comparación de agentes (F11.2 / NOV-88)

Ejecuta cualquier agente registrado individualmente y compara
agentes con la misma tarea sin lanzar el corpus completo. El Lab no
duplica nada: reusa fixtures, splits y outcomes de `agent-tasks`
(`internal/bench`), enlaza resultados a trazas F11.1
(`internal/observability` + `trace timeline`) y no implementa routing
automático (F12.1).

- Código: `internal/bench/agentlab*.go` (registry, config, ejecución,
  comparación, evidencia), `internal/cli/bench_lab.go` (comandos).
- Tests: aislamiento, replay y cancelación en `agentlab_test.go`;
  rutas CLI con host falso en `bench_lab_test.go`.

## Agentes registrados (`bench lab agents`)

| Agente | Responsabilidad | Skill | Host |
|---|---|---|---|
| `baseline` | Resolver con el prompt visible, sin andamiaje (control) | (none — control) | opencode |
| `anchored` | Resolver con anclaje de workdir, disciplina de tests y autoverificación | regulus-verify (verify subset) | opencode |
| `verify-first` | Reproducir el fallo con los checks visibles, luego fix mínimo y verificar | @regulus-verifier protocol | opencode |

Cada entrada declara además herramientas (`read, edit, bash`) e
inputs (`task.prompt, fixture.files, workdir`). El registry crece por
host (claude/codex/devin) sobre la misma interfaz `HeadlessAgent`;
comparar es contrastar, nunca rutear.

## Modos

```sh
# Individual: un agente, una fixture, host/modelo y presupuesto
bin/regulus bench lab run --agent anchored --task AT4 --model M --budget-s 60

# Comparativo: misma tarea, fixture, criterios y límites, lado a lado
bin/regulus bench lab compare --agents baseline,anchored --task AT4

# Integrado: run + reconstrucción F11.1 inline (sigue el flujo completo)
bin/regulus bench lab flow --agent verify-first --task AT4

# Inventario y reporte desde evidencia verificada
bin/regulus bench lab agents [--json]
bin/regulus bench lab report --evidence-dir <dir>
```

Ningún modo exige el corpus: `--task` es siempre una sola fixture
validada contra los mismos splits versionados (`REGULUS_AGENT_SPLITS`
o `bench/agent-splits-v3.json`); exploración nunca toca holdout.

Selección común: `--host` (override del registry), `--model`,
`--host-version`, `--budget-s` (solo baja el techo de la tarea),
`--seed` (defecto: seed de splits), `--retries` (0..5, solo infra
transitoria), `--variant`, `--export F` (guarda la config
replayable), `--evidence-dir D`. Con `--config F` la ejecución repite
exactamente esa configuración: cualquier flag de selección al lado
es error de uso (exit 2).

## Aislamiento por ejecución

- Workspace fresco (`lab-work-*`, 0700) con la fixture (0600);
  se elimina al terminar — el reporte registra la ruta y `workdir
  cleaned`. Dos ejecuciones nunca comparten workspace, outputs,
  `run_id`, `agent_execution_id` ni `session_id`.
- Check con entorno filtrado (forma de la allowlist de
  `internal/gates`, sin tocarla): toolchain, locale, proxy y
  namespaces harness/CI pasan; `FIXTURE_DIR`, `LAB_RUN_ID` y
  `LAB_SESSION` se añaden al final; secretos y resto se caen.
- El host hereda el proceso (binario externo); lo evaluado — el
  check — corre enjaulado con `cwd` en el workspace y timeout por
  presupuesto.

## Reporte y clasificación

Cada resultado trae: instrucciones exactas enviadas, metadata
recibida, event ids F11.1, herramientas observadas (el host no
expone ninguna: se reporta `[]` con procedencia `unknown`, nunca
inventadas), delegaciones (vacío: el Lab no delega), duración,
tokens con procedencia (`measured-host` o `unknown`, nunca sumados
como medidos), reintentos, errores, cambios (añadidos/modificados
vs borrados; `go.mod` y artefactos `zz_hidden*` son del runner, no
del agente) y veredicto.

Clasificación: `pass | fail | timeout | infra-error | inconclusive |
not-run`. Solo pass/fail son evaluables; infra/timeout nunca pasan;
cancelación es `not-run`. Exit CLI: run/flow 0 (pass/fail), 1
INCONCLUSIVE, 3 NOT RUN; compare 0 con ≥1 evaluable (imprime
`winner <id|tie>`), 1/3 igual.

## Trazas F11.1 y replay

Cada ejecución emite al store global (`run.start, agent.start,
context, [retry|timeout], agent.end, run.end`) con los mismos IDs
correlacionables; el reporte incluye `trace: regulus trace timeline
--run <id> --rebuild` y `flow` lo reconstruye inline (veredicto,
delegaciones, evidencia malformada/duplicada). Sin root de proyecto
el tracing se omite (runs herméticos de test).

```sh
# Exportar y repetir bit a bit (mismo veredicto con el mismo host)
bin/regulus bench lab run --agent anchored --task AT4 --export lab.json
bin/regulus bench lab run --config lab.json

# Evidencia: config.json + outcomes.json + manifest.json (sha256) + reports.json
bin/regulus bench lab run --agent anchored --task AT4 --evidence-dir lab-ev/
bin/regulus bench lab report --evidence-dir lab-ev/   # verifica sha antes de rendir
```
