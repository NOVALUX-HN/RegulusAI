# Contracts (NOV-37)

Three version families, pinned with explicit refuse (no silent fallback):

- plugin↔core contract: `regulus.contract/v1` (`internal/contract`; `Supported`, `Supports`)
- MCP contract: `regulus.mcp/v1` (`internal/mcp`; `SupportedContracts`, `SupportsContract`, `NegotiateVersion`)
- MCP wire protocol: `2024-11-05` (`internal/mcp`; `SupportedProtocols`, `SupportsProtocol`, `NegotiateProtocol`)

Policy: optionals stay optional; defaults never change meaning; unknown
versions error naming the supported set (`manifest check` fails with
`unsupported manifest version N: version 1 maps to …`). tools.json
`version: 1` means exactly the three pinned values above.
Deprecation: add the new version to `Supported*`, migrate callers, then
remove the old in a reviewed bump — never drift silently.
