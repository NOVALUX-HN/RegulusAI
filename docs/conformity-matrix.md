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
