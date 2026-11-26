# Conformidad multiagente (NOV-55 / T29, niveles NOV-69)

Niveles por capacidad y host, generados desde resultados (`regulus bench host-journeys`):
`declarado` (documentado) → `contrato-probado` (contrato + smoke CLI) →
`host-probado` (journey ejecutado en el host real con evidencia enlazada).
Sin host o sin credenciales el job marca `no-ejecutado`, nunca `pass`;
el fallo de un host no degrada el core ni los otros adaptadores.
Programado: `.github/workflows/host-conformity.yml` (lunes 04:37 UTC +
manual, 20 min acotados, artefactos 90 días). Verificación en host
real preserva configuración ajena (backups antes de escribir,
`internal/hostcfg`: lo no administrado se informa y se detiene).

Journeys por superficie declarada (instalación, doctor, contexto/memoria, spec/Chart, gate, receipts, errores, permisos, cierre). Sin hosts instalados: pruebas contractuales + smoke CLI.

Verificado local:
- `go test ./internal/contract ./internal/mcp ./internal/skills` (vía `C:/Temp/*.test.exe` por App Control Temp, ver NOV-43) PASS.
- `regulus contract` → `regulus.contract/v1`; `regulus doctor`; `regulus skills list` (registry sin duplicados, precedencia proyecto>global).
- OpenCode: `bunx tsc --noEmit`, `bun test ./lib/` (plugin TS).
- Codex: `AGENTS.md` + `.agents/skills/regulus` + `.codex/config.toml` (MCP stdio); sin OpenCode/Bun.
- Claude: `CLAUDE.md` + `.claude/skills` + `.claude/agents` + `.mcp.json`.
- Devin: `.devin/` pack + `scripts/install-devin.*` + `docs/devin.md` matriz CLI/Desktop/Cloud.

Matriz (solo capacidades verificadas; fallo de un host no degrada core):
- OpenCode V2: full (hooks, 19 tools, agents).
- Codex CLI: MCP + skills + CLI; hooks/subagentes solo si versión los soporta, si no inline degradado explícito.
- Claude Code: MCP + skills + agents + hooks opt-in; sin mutar permisos sin consentimiento.
- Devin CLI/Desktop: full vía MCP + skills; Cloud: sin persistencia disco (export/import explícito), blueprint Windows `runs-on:windows`.

Windows PowerShell/paths probados en CI `windows-native`; Devin Cloud Windows ≠ Windows local (ver docs/devin.md).

Estado de hosts 2026-11-14 (verificado a mano, sin modelo usable):
- OpenCode v2.2.5: binario presente, sin créditos de modelo (`run` → 402 quota) → journeys `no-ejecutado`.
- Claude Code: binario presente, sin login → `no-ejecutado`.
- Codex: sin binario CLI → `no-ejecutado`.
- Devin: solo app Desktop, sin modo no interactivo → `no-ejecutado`.
Journey de referencia ejecutado con binario real en fixture (`install`, `doctor` advisory, `context`, `spec`, `gate --yes`, `close` en verde con receipts).

Estado de hosts 2026-11-16 — NOV-76 Paso 54 (Windows, `windows:true`):
- Comando: `bin/regulus.exe bench host-journeys --run --out .regulus/host-evidence-run` → `host-journeys PASS: 3 host(s) green` (exit 0).
- Evidencia: `.regulus/host-evidence-run/host-journeys-20261116T005418Z.json` (sha256 `d6cc7dd86353931364f3198250fa4fdcc0707fd7b879b343660fa05dc8b953d3`), probe previo `.regulus/host-evidence-probe/host-journeys-20261116T005408Z.json` (sha256 `a1a553d674e274fecbb13b7fa94609ad9f13396430db888a673724f2fea16a67`).
- OpenCode v2.2.5: `host-probado` / `pass` — install, doctor, context, spec, gate, receipt and close green.
- Claude 2.1.325 (Claude Code): `host-probado` / `pass` — mismo journey en verde.
- Codex codex-cli 0.169.0: `host-probado` / `pass` — mismo journey en verde (binario ahora presente, antes ausente).
- Devin: `no-ejecutado` / `not-run` — `devin ships desktop-only, no non-interactive CLI` (verificado 2026-11-16, `internal/bench/hostjourney.go:83`, `bench_host.go:75`; sin API/CLI no-interactiva conocida).
- Preservación config ajena (host real, no solo fixtures): `go test ./internal/hostcfg -run TestInspectClassifies|TestForeignJSONCPreservedByteForByte` PASS — `hostcfg.go:19 Marker`, `:32 Inspect Missing/Managed/Foreign`, `:57 Backup .bak-*`, `:75 Install rehúsa foreign sin tocar bytes`, `:117 Uninstall solo managed`; gap documentado: sin test de permisos ACL/mode ni merge MCP terceros con `~/.claude.json` real (solo fixture genérico, no importado por CLI).
- Job programado: `.github/workflows/host-conformity.yml` cron `37 4 * * 1` + `workflow_dispatch`, `timeout 20m`, artefacto `host-journeys` 90d; corrida local 2026-11-16 documentada arriba; corrida CI pendiente (`gh workflow run host-conformity.yml`, requiere push + `HOST_API_KEY` para `--run`, hoy solo probe sin costo en CI).
- Fragilidad: `toolVersion 30s`, `5min/step`, job `20min`, sin reintentos, `latency_ms:0` no medido; `isQuotaErr` + `classifyJourneyErr` (quota/auth → `not-run`, `doctor` advisory → `not-run`, resto → `fail` con paso); `AgentInfra` solo cubre MkdirTemp/Fixture — quota/auth colapsan a `not-run`, `doctor` puede enmascarar fallo real.

## F11.7 · Conformidad multi-host y matriz de capacidades (NOV-93)

Sustituye afirmaciones generales de compatibilidad por journeys
ejecutados y capabilities verificadas. Comando:
`regulus bench host-capabilities [--run] [--model M] [--out DIR] [--host all|opencode|claude|codex|devin]`
(`internal/cli/bench_hostcap.go`, manifiestos en
`internal/bench/hostcapabilities.go`, snapshots en
`internal/hostcfg/evidence.go`).

### Reconciliación NOV-69 / NOV-76 (qué se reutiliza, qué es nuevo)

| Pieza | Origen | Estado F11.7 |
|-------|--------|--------------|
| Journey install/doctor/context/spec/gate/close + niveles declarado→contrato-probado→host-probado | NOV-69 (`internal/bench/hostjourney.go`, `bench_host.go`) | Reutilizado sin cambios: `host-capabilities --run` ejecuta `executeHostJourney` y pliega el outcome vía `ApplyJourneyOutcome` |
| 3 hosts host-probado con evidencia enlazada (2026-11-16) | NOV-76 | Reutilizado como evidencia fechada (`JourneyEvidence`/`JourneyDate`); un probe posterior no la degrada, solo añade `ProbeNote` |
| `hostcfg` ownership (marker, backup, foreign intacto) | NOV-69 S4 | Reutilizado + extendido con snapshots redactados read-only (`SnapshotForEvidence`, `ConfigRefs`) |
| Exec log + `trace timeline` | F11.1 | Reutilizado: cada corrida emite `run.start` + `task.end` por host + `run.end` y enlaza `regulus trace timeline --run <id> --rebuild` |
| Capability manifest 12 dimensiones + matriz de estados + degradación + distinción probe/journey | F11.7 | Nuevo (`hostcapabilities.go`, `bench_hostcap.go`, `hostcfg/evidence.go`); F16.2 amplió el vocabulario a ocho estados y agregó `ValidateEvidenceCoverage` |

### Matriz resultante (2026-11-16, `bench host-capabilities --host all`, clase probe)

| Capacidad | opencode | claude | codex | devin |
| --- | --- | --- | --- | --- |
| tools | probada | probada | probada | no-ejecutada |
| hooks | soportada | soportada | inferida | no-soportada |
| subagents | soportada | soportada | inferida | no-ejecutada |
| mcp | probada | probada | probada | no-ejecutada |
| sessions | probada | probada | probada | no-ejecutada |
| tokens | probada | probada | probada | desconocida |
| filesystem | probada | probada | probada | no-soportada |
| permissions | soportada | soportada | soportada | no-soportada |
| limits | soportada | soportada | soportada | desconocida |
| install | probada | probada | probada | no-ejecutada |
| gate | probada | probada | probada | no-ejecutada |
| close | probada | probada | probada | no-ejecutada |

Estados posibles: `probada` / `soportada` / `inferida` / `parcial` /
`degradada` / `no-soportada` / `no-ejecutada` / `desconocida`
(validados por `ValidateCapabilitiesAttributable`; palabra fuera del
vocabulario = exit 1). `probada` es la única que afirma una
observación: exige una corrida que ejercite la dimensión y una cita que
la cubra (`ValidateEvidenceCoverage`). Lo que el contrato declara sin
observación es `soportada`; lo derivado de una señal, como un gate de
versión, es `inferida`. Host desconocido → `desconocida`, nunca
pass. Versiones vivas 2026-11-16: OpenCode v2.2.5, Claude 2.1.325
(Claude Code), Codex codex-cli 0.169.0, Devin desktop-only (sin CLI).

### Journeys reales por host + estrategia Devin

- OpenCode: `npm install -g opencode-ai@1.19.5` → `opencode --version` → `bench host-journeys --run --host opencode` (fixture E2E, sin costo de modelo) → agent-leg `opencode run --model <m> "<prompt>"` (credencial-gated). Repetible en CI: sí.
- Codex: `npm install -g @openai/codex@0.169.0` → `codex --version` → `bench host-journeys --run --host codex` → agent-leg `codex exec --model <m> "<prompt>"` (credencial-gated). Repetible en CI: sí (pins con la versión verificada 2026-11-16; si el registro renombra el paquete, el probe lo informa como no-ejecutado, nunca pass).
- Claude: `npm install -g @anthropic-ai/claude-code` → `claude --version` → `bench host-journeys --run --host claude` → agent-leg `claude -p "<prompt>"` (requiere login). Repetible en CI: fixture sí, agent-leg no (login).
- Devin (estrategia realista, sin CLI no-interactiva conocida): 1) exportar pack `.devin/` del repo, 2) sesión Desktop/Cloud con install/doctor/context/spec/gate/close, 3) importar la evidencia a `host-evidence/` (nunca inventarla). Clase `strategy`, nunca ejecuta, nunca CI-repetible.

Mínimo "al menos 2 hosts E2E repetibles en CI": OpenCode + Codex
(fixture journey sin credenciales, binarios pineados).

### CI: probe vs journey (nunca etiquetar probe como journey)

La clase de ejecución viaja con el run (`execution_class` en el
banner, en cada `task.end` y en el JSON), nunca se infiere de las
capabilities: `host-capabilities` = probe (sin costo), con `--run` =
journey, agent-leg = `agent-journey` credencial-gated
(`HOST_API_KEY`), Devin = `strategy`. `ValidateNoProbeAsJourney`
falla el comando si un manifest clase probe reclama capabilities
cubiertas como probadas, y `ValidateEvidenceCoverage` falla el comando
si cualquier manifest — probe o journey — cita el journey para una
dimensión que `JourneyCoveredCaps` no cubre. Workflow `.github/workflows/host-conformity.yml`
sin cambios: pata probe siempre verde + pata `--run` solo con
`HOST_API_KEY` (hoy la pata de agente se registra, no se ejecuta
contra el host, como en NOV-72).

### Versiones, config, capabilities, comandos, artifacts

El pack `--out host-capabilities-<ts>.json` registra por host
versión viva, clase, 12 capabilities con evidencia fechada, comandos
probe/journey/agent, `config` (solo rutas + estado
managed/foreign/missing vía `hostcfg.Inspect`, contenidos jamás) y
`artifacts` redactados. Choke point: `bench.NewHostEvidence`
(`RedactMap`/`RedactText` de observability); snapshots de ficheros
vía `hostcfg.SnapshotForEvidence` (read-only, tope 8KB +
`[truncado]`, secretos → `[redacted]`). Secretos fuera de artifacts:
testeado (`TestNewHostEvidenceRedactsSecrets`,
`TestSnapshotForEvidenceRedacts`, `TestConfigRefsNeverStoresContents`).

### Degradación controlada

Codex hooks/subagentes version-gated → inline degradado explícito;
Claude hooks opt-in y permisos sin consentimiento → el journey se
detiene y reporta no-ejecutado; Devin filesystem Cloud → export/import
explícito; resto Devin → estrategia manual con evidencia importada.
El fallo de un host nunca degrada los otros (`DegradationFor`,
testeado por host×capacidad).

### Trazas F11.1 enlazadas

Cada corrida emite al exec store (`run.start` + `task.end` por host
con `capability_class`/`execution_class` + `run.end`) e imprime
`traza: regulus trace timeline --run <id> --rebuild` por host y al
cierre (mismo formato que el lab, sin store paralelo).
