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
