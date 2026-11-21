# Verificación continua (F13.4 / NOV-107)

Verificar **cada cambio y cada integración durante la misión**, no
solo al final: plan por tarea y por misión desde el diff, ejecución
con veredictos cerrados, gates por riesgo, regresiones F12.4
vinculadas al cambio, evidencia tamper-evident y rerun reproducible.

## Reúso (nada paralelo, nada duplicado)

| Necesidad | Fuente reutilizada |
|---|---|
| Grafo, aristas verificadas, diff→consumidores | `impact.BuildGraph/MapDiff` (F12.8): verify no recorre el repo |
| Riesgo explicable | `impact.AssessRisk` (F12.8): verify solo añade la escalada |
| Foco + precisión/recall | `impact.SelectSuite/MeasureSelection` (F12.8): verify no redefine |
| Tiers L0–L3, recomendación, techos | `bench.RecommendTier/FindTierSpec/TierSpecs` (F11.3): nunca L3 por foco |
| Baselines, detección N-adecuada | `quality.Check/ReadBaselineEvidence/LoadWindow` (F12.4): verify no reimplementa |
| Traza run.start+task.end+run.end | `observability` vía `recordImpactTrace` (F11.1, best-effort) |
| Run id | `observability.MintRunID` (el edge acuña, el motor nunca inventa identidad) |

Vocabulario propio (no reutiliza `verdict.Status` a propósito: trae
4 estados y la spec exige 6): **PASS, FAIL, BLOCKED, SKIPPED,
UNKNOWN, NOT_APPLICABLE**. Niveles: low/medium/high (espejan
`impact.Risk.Level`) + **critical** (escalada verify: breaking o
incertidumbre apilada).

## Matriz riesgo→gates

```
low      → build, static, unit                          [tier mínimo L0]
medium   → + integration                                [tier mínimo L1]
high     → + e2e, security                             [tier mínimo L2]
critical → + mutation                                  [tier mínimo L2]
```

Escalas que **solo suben** (la incertidumbre suma, nunca resta):

- `widened` (entrada sin nodo verificado → suite completa): +1 nivel.
- severidad F12.4 observada ≥ 0.50: +1 nivel.
- contratos tocados: **high mínimo** (e2e+security obligatorios).
- breaking (`breaking:` o `!` en el contrato): **critical** (toda la matriz).

`regulus verify matrix` imprime la matriz generada desde el mismo
código que gatea (tabla y motor no pueden divergir).

## Cuándo se amplía la suite

1. **Sin nodo verificado** (`impact.Widened`): el diff no mapea a
   nada conocido → corre la **suite completa** y el riesgo sube un nivel.
2. **Nodo crítico en el camino** (kernel `gates/evidence/receipts/
   manifest/verdict/contract` o contratos tocados): los tests
   críticos **siempre van en el foco**; con breaking se corre todo.
3. **Override débil rechazado**: `--tier` por debajo del necesario
   con widened/críticos/breaking **se rehúsa** (exit 1) en vez de
   omitir en silencio — una selección predictiva débil nunca omite
   pruebas críticas.
4. **Severidad F12.4 alta** (≥ 0.50): +1 nivel y piso de tier mayor.
5. **L3 nunca por foco**: el holdout queda sellado (solo periódica/
   pre-release); `--tier L3` se rehúsa.

El contraste se conserva siempre: el reporte cita foco vs full con
**precisión/recall/f1** (`impact.MeasureSelection`), corra lo que corra.

## Comandos

```sh
# Plan por tarea/misión desde diff+impacto+contratos+riesgo+criterios
regulus verify plan --changed <a,b> [--task <t>] [--mission <m>] \
  [--severity N] [--contracts <c,...>] [--criteria <c,...>] \
  [--tier L0|L1|L2] [--out <dir>] [--json]

# Ejecutar: outcomes registrados (flags/archivo), nunca inventados
regulus verify run --changed <a,b> [mismo plan...] \
  [--fail gate:causa,...] [--skip ...] [--block ...] [--na ...] \
  [--results <file>] [--no-toolchain] \
  [--baseline <dir> --window <file>] [--out <dir>] [--json]

# Reporte por tarea/misión desde evidencia guardada
regulus verify report --dir <dir> [--json]

# Matriz riesgo→gates (generada del motor)
regulus verify matrix [--json]

# Rerun reproducible a un dir NUEVO (nunca sobrescribe)
regulus verify rerun --dir <dir> --out <nuevo-dir> [--fail ...] [--json]
```

Códigos: 0 allow (o reporte/matriz ok), 1 bloqueo/refusal
(incluye evidencia tampered: sha mismatch), 2 uso.

## Veredictos y gateo

- **FAIL** en un gate requerido → `FAIL / block`: merge,
  integración y transición de misión **bloqueadas**.
- **BLOCKED / SKIPPED / UNKNOWN** en un gate requerido → bloquean
  con ese veredicto como overall. **Nunca son PASS** (pinneado por test).
- **NOT_APPLICABLE con causa** → no bloquea (exclusión explícita,
  p. ej. "sin tests de integración en el foco"), nunca pasa en silencio.
- Gates no requeridos (advisory) se registran y no gatean.
- Sin toolchain (`go` ausente o `--no-toolchain`): todo gate
  **BLOCKED** (multi-host seguro). Un PASS registrado que contradiga
  un toolchain ausente degrada a BLOCKED.
- Sin evidencia registrada el gate queda **UNKNOWN** → bloquea
  (aporte `--fail/--skip/--block/--na` o `--results`).
- Regresión F12.4 con alertas → `FAIL / block` vinculado al diff
  (magnitudes + run ids + limitaciones en el reporte).

## Regresiones vía F12.4

`run --baseline <dir> --window <file>` pliega el diagnóstico
`quality.Check` al gate: el reporte muestra **baseline (n),
muestra (n, excluidos), magnitudes, casos vinculados y
limitaciones**. Con `n < 5` el detector describe y no afirma
(`inconcluyente`); con ventana mezclada cita el confound por
segmento. Detección ⇒ bloqueo de la integración del diff
responsable (commit + diff hash + run id en la causa).

## Evidencia y rerun

- Dir de plan: `plan.json` + `manifest.json` (sha256 del plan).
- Dir de resultado: `plan.json` + `result.json` + `manifest.json`
  (ambos shas + decisión). Autocontenido: report y rerun
  reproducen desde estos bytes.
- Lecturas verifican shas primero: lo editado **falla cerrado**.
- El rerun escribe siempre en un dir nuevo (`--out == --dir` o dir
  con evidencia se rehúsa), conserva la config (misma `config_id`)
  con run id fresco, y compara: `Comparison` muestra `mismo plan`,
  movimientos por gate y `overall/decisión antes → después`.

## Corpus de validación

`internal/verify/testdata/validation-corpus.json`: 11 escenarios
(riesgo bajo, fallo inyectado, crítico, contratos, breaking,
widened, multi-paquete, BLOCKED/SKIPPED/UNKNOWN nunca PASS,
NOT_APPLICABLE) con riesgo, gates y decisión esperados.
`TestValidationCorpus` los ejecuta sobre el fixture grafo y exige
la decisión exacta: editar un escenario exige actualizar el test.

## Límites

- El pin de dispatch (`run_split_test.go`) y la línea de `usage`
  en `cli.go` los actualiza el parent: `verify` se registra por
  `init()` sin tocar `cli.go` (igual que `quality` e `impact`).
- `verify run` no inventa ejecuciones: sin outcomes registrados los
  gates quedan UNKNOWN y bloquean. La ejecución real vive en los
  gates existentes (`gate run`) y sus receipts alimentan
  `--results`.
- El módulo propone y gatea desde evidencia; no muta producción.
