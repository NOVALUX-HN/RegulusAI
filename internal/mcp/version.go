package mcp

import "fmt"

// ContractVersion pins the MCP interface contract. Like
// internal/contract, it evolves only through explicit versions:
// negotiation accepts the exact pinned value and refuses anything else.
const ContractVersion = "regulus.mcp/v1"

// NegotiateVersion accepts the exact pinned contract version and refuses
// anything else. Compatibility is explicit, never silent drift.
func NegotiateVersion(client string) (string, error) {
	if client == ContractVersion {
		return ContractVersion, nil
	}
	return "", fmt.Errorf("mcp contract mismatch: client speaks %q but server speaks %q", client, ContractVersion)
}
