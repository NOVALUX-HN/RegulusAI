# Gate de deriva documental (NOV-63)

`regulus manifest check` extiende la comparación de tools con cinco
chequeos derivados de fuente (sin gate nuevo: el conteo del gauntlet
sigue en 15 y `bench/gauntlet-shape.json` intacto):

1. **Versión**: `package.json` == `versioninfo.json` (Fixed y String) == `regulus.manifest` == head de `CHANGELOG.md` == línea `Status:` del README.
2. **Gates**: ocurrencias de `N-gate` / `N layers` en README y workflows == nº de gates en `regulus.json`.
3. **Tools**: `N tools` con sabor MCP == `len(mcp.ToolNames())`; con sabor plugin == registros `add({` en `.opencode/plugins/regulus.ts`.
4. **Runtimes**: ningún `only supported runtime` mientras existan adaptadores multi-host (ver `docs/conformity-matrix.md`).
5. **Comandos**: el README no nombra comandos inexistentes (derivados del texto de `usage`, que incluye `serve` de `cmd/regulus`).

## Documentos históricos (H10)

Un `.md` con el marcador `H10` es un registro congelado y se excluye.
Para marcar uno nuevo, añade al inicio una línea como:

`> **Nota (H10):** frozen <qué> (<fecha>); ...`

Excluidos hoy: `docs/token-savings.md`, `docs/PRD.md`, `docs/audit/repository-inventory.md`.
