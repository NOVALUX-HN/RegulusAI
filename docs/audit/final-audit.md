# RegulusAI — Final Audit & Fix

Fecha: 2026-10-31. Rama: `develop` (limpio). Alcance: phases 0–8 sobre
`origin/main` (0.9.0 funcional). Regla: corregir lo verificable de forma
segura; sin commits/push/releases (el árbol queda con los cambios sin
commitear, listados abajo).

## Línea base (evidencia, no declaración)

- `go vet ./...`: sin diagnósticos. `go build ./...`: ok.
- `go test ./...`: 28 paquetes `ok`, 0 FAIL (incl. `journey`, la suite de
  integración, y `internal/observability`, `runtime`, `mcp`, `httpapi`).
- Plugin TS (`cd .opencode && bun test`): 56 pass, 0 fail, 620 expects.
- Smoke vivo con binario compilado (`/tmp`, fuera del repo):
  `regulus contract` → `regulus.contract/v1`;
  `regulus status` → datos reales (opencode v2.2.3, 12 gates, 569 receipts);
  `regulus mcp` + `initialize` → `protocolVersion 2024-11-05`,
  `serverInfo.contract regulus.contract/v1`,
  `serverInfo.mcpContract regulus.mcp/v1`.
- `go.mod`: cero dependencias externas (pineado por test).
- Secrets scan (api_key/secret/bearer/credentials en Go+TS, excluyendo
  tests/redacción): limpio.

## Problemas encontrados y severidad

| # | Severidad | Hallazgo | Acción |
|---|---|---|---|
| 1 | Baja (docs) | `docs/observability.md` §3 no admitía el gap de guardian-disagreement (reporta 0 con coverage note, pero el doc no lo decía) | CORREGIDO en árbol (nota de honestidad agregada; tests ya lo pineaban) |
| 2 | Baja (deuda) | `internal/cli/mcp.go`: const `mcpProtocolVersion` en desuso real (initialize usa `mcp.ProtocolVersion`); solo la lee un test de paridad | No tocado: el test lo pinea a propósito; quitarla exige editar test + wiring sin beneficio funcional |
| 3 | Baja (deuda) | `internal/runtime/registry.go`: `Register` hace `panic` en mal uso | No tocado: guard fail-fast idiomático para error de programación, ruta no testeada pero tampoco alcanzable en producción (registro vacío) |
| 4 | Baja (deuda) | `internal/httpapi/envelope.go`: `_ = json.NewEncoder(w).Encode(env)` descarta error de escritura HTTP | No tocado: el handler no retorna error y en la práctica solo falla con conexión rota; refactor requeriría firma nueva sin ganancia |
| 5 | Baja (diseño) | `Negotiate` es exact-match case-sensitive y propaga duplicados | No tocado: uso actual es membership; endurecerlo es cosmético |
| 6 | Media (diseño) | Experimentos de evolution no consumen el cap global diario (`budget.go`, 10/día): cada invocación está acotada (`RunCap`/`MaxRuns`) pero muchas invocaciones manuales eluden el ledger | NO rewireado en auditoría: requiere decisión de producto (nuevo `Kind` + qué cuenta como consumo); las corridas son invocadas por operador, no autónomas → sin autoevolución irrestricta real |
| 7 | Baja (alcance) | `internal/httpapi` es librería sin entrypoint CLI (`regulus serve` no existe) | No es bug: S5 pedía evaluar + definir contratos, y los docs no prometen comando; anotado como trabajo futuro explícito si se quiere servir HTTP |

Críticos: ninguno. Altos: ninguno.

## Correcciones realizadas (verificadas)

- `docs/observability.md` §3: nota del gap guardian-disagreement.
  Suite `internal/observability` re-ejecutada: `ok` (sin impacto, cambio
  solo-docs).
- Sin desactivar ni editar tests para conseguir verdes: los `t.Skip`
  existentes son condicionales de entorno con motivo (`git not available`,
  `gremlins is not installed` + dogfood referenciado). Los `panic` fuera
  de tests son solo el guard de `Register` y el `os.Exit` de `main`.

## Integración verificada

- Core ↔ módulos: `MCP ↔ CLI` comparten paths por buffers (testeado por
  paridad + smoke vivo); `httpapi` delega a los mismos core paths
  (consistencia testeada CLI↔HTTP).
- Adaptadores: plugin TS preservado (56/56 + pin `REGULUS_CONTRACT` ==
  `regulus.contract/v1` verificado en suite Go y en vivo).
- Contratos versionados y pineados: `regulus.contract/v1`,
  `regulus.mcp/v1`, `regulus.http/v1`; versiones desconocidas se rehúsan
  (testeado).
- Persistencia: `.regulus/` gitignored; escrituras atómicas donde importa
  (`budget` tmp+rename, `Prune` tmp+rename, `O_EXCL` en stores); los
  `WriteFile` directos de evolve siguen el patrón preexistente de
  `decisions` (riesgo de escritura parcial aceptado en todo el proyecto,
  blast radius de cambiarlo ≫ beneficio).

## Docs vs realidad (spot checks)

- `docs/architecture.md` §11: verificado contra código (paquetes, pins,
  endpoints como librería). OK.
- `docs/evolution.md` + ADRs T6: consistentes con `internal/evolve`. OK.
- `docs/observability.md`: corregido el gap del punto 1. OK tras fix.
- CHANGELOG `[Unreleased]`: refleja phases 6–8. OK.

## Pendiente y deuda técnica

1. Decisión producto: ¿los experimentos consumen el cap global de
   evolution? (punto 6).
2. Opcional: entrypoint `regulus serve` para `httpapi` (punto 7).
3. Limpiezas menores cuando se toque el área: const `mcpProtocolVersion`
   (2), `_ = Encode` (4), `Negotiate` estricto (5).
4. Phase 8 sin pushear: `develop` lleva 5 commits sobre `origin/develop`
   (fases 6–8 ya subidas antes; lo nuevo es phase 8 + este informe sin
   commitear por regla de la tarea).

## Cambios en el árbol (sin commit, por regla)

- `M docs/observability.md` (fix punto 1).
- Nuevo `docs/audit/final-audit.md` (este informe).
