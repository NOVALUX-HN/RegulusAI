package mcp

import "fmt"

// ContractVersion pins the MCP interface contract. Like
// internal/contract, it evolves only through explicit versions:
// negotiation accepts the exact pinned value and refuses anything else.
const ContractVersion = "regulus.mcp/v1"

// SupportedContracts lists every MCP contract this server speaks.
var SupportedContracts = []string{ContractVersion}

// SupportedProtocols lists every MCP wire version this server speaks.
var SupportedProtocols = []string{ProtocolVersion}

// SupportsContract reports whether v is a supported MCP contract.
func SupportsContract(v string) bool {
	for _, s := range SupportedContracts {
		if v == s {
			return true
		}
	}
	return false
}

// SupportsProtocol reports whether v is a supported MCP wire version.
func SupportsProtocol(v string) bool {
	for _, s := range SupportedProtocols {
		if v == s {
			return true
		}
	}
	return false
}

// NegotiateVersion accepts a supported contract version and refuses
// anything else. Compatibility is explicit, never silent drift.
func NegotiateVersion(client string) (string, error) {
	if SupportsContract(client) {
		return client, nil
	}
	return "", fmt.Errorf("mcp contract mismatch: client speaks %q but server speaks %q", client, ContractVersion)
}

// NegotiateProtocol accepts a supported wire version and refuses anything
// else — same explicit-refuse policy as contracts, no silent fallback.
func NegotiateProtocol(client string) (string, error) {
	if SupportsProtocol(client) {
		return client, nil
	}
	return "", fmt.Errorf("mcp protocol mismatch: client speaks %q but server speaks %q", client, ProtocolVersion)
}
