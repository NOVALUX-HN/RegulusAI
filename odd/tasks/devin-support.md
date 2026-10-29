# devin-support — Regulus inside Devin (CLI, Desktop, Cloud)

One MCP server plus one `.devin/` pack serves all three Devin surfaces
(CLI local, Desktop native, Cloud Windows VMs) — no OpenCode runtime
needed anywhere. Desktop live testing is the owner's step after D3.
Read down to `## Log`.

## Specs

- S1: "ya que tenemos todo lo que se requiere para poder usar Regulus en Devin comencemos con el desarrollo. hazlo de forma ordenada y trabaja probando para que funcione en esa herramienta. cuando termines todo me avisas para probar tambien en el desktop" — full authorization, orderly, every unit tested (protocol round-trips, pack validation, full suite); desktop testing stays the owner's step.
- S2: "me gustaria que Regulus se pueda usar en Devin, investiga que se requiere para poder instalarlo y hacerlo funcionar en ese software y que tambien pueda usar todos los plugins como Almanac y el resto" — `regulus mcp` (stdio JSON-RPC, zero new go.mod deps) exposes Almanac (memory_note/show/save/search/context/history) plus gate_run, spec_check, doctor, trace_show/sessions/files. Deliberately OUT with reasons: probe_contrast/compose (2-3x gate cost per call), evolve_propose (harness self-modification doesn't transfer), skills_list (opencode skill registry; Devin has its own), memory prune/import/export (destructive or management ops; prune refuses silently destructive use — Devin-side policy, not a protocol fit).
- S3: "sobre Devin, regulus debe funcionar en tanto en su CLI como en su version desktop" — stdio MCP + `.devin/` pack is surface-agnostic (CLI mcp_config, Desktop native, Cloud blueprint); Windows paths documented (%APPDATA%); Cloud caveats documented (Ubuntu default, Windows runs-on +9% and limited availability, .regulus/ non-persistence across sessions).
- S4 (invariants): single Go binary stays trivially buildable (CGO-free, no new deps); MCP speaks newline-delimited JSON-RPC 2.0; no secrets committed; `.regulus/` stays gitignored.

## Tasks

- [x] D1 (S2,S4) — `regulus mcp` stdio server + 12 tools + protocol tests — route: worker — commit: 21e32a2
- [x] D2 (S2,S3) — `.devin/` pack (regulus-workflow skill, worker+scout agents, mcp_config, permission snippet doc) + pack validation tests — route: worker — commit: 376edad
- [x] D3 (S1,S3) — docs/devin.md (Windows blueprint copy-paste, CLI/Desktop/Cloud recipe, limits) + full verification + reviewable commits — route: inline — commit: this unit
- L2 (D3 verification): full `go test ./...` green (incl. journey), bun 38/38, tsc/vet/gofmt clean, `go list -m all` = stdlib only; blueprint YAML parses (`runs-on: windows`, 2 init steps); `.devin/mcp_config.json` wires `regulus mcp`; local bins restamped 0.8.1 with the mcp subcommand.

## Log

- L1: Origin — "el segundo es que me gustaria que Regulus se pueda usar en Devin, investiga que se requiere para poder instalarlo y hacerlo funcionar en ese software y que tambien pueda usar todos los plugins como Almanac y el resto" → research: Devin CLI (.devin/ skills/agents/MCP/permissions), Devin Desktop (Windsurf lineage, native Windows installer), Devin Cloud (Ubuntu 22.04 default, Windows runs-on + shell + choco, setup-go action, +9% usage, limited availability). "sobre Devin, regulus debe funcionar en tanto en su CLI como en su version desktop. por eso quiero que investigues a fondo como hacerlo." → decision: one stdio MCP server + one committed `.devin/` pack covers all three; the opencode plugin (hooks/16 tools/agents) does NOT transfer (needs OpenCode runtime) and is NOT ported tool-for-tool. "ya que tenemos todo lo que se requiere para poder usar Regulus en Devin comencemos con el desarrollo. hazlo de forma ordenada y trabaja probando para que funcione en esa herramienta. cuando termines todo me avisas para probar tambien en el desktop".
