package mcp

// ProtocolVersion is the MCP wire version served. It mirrors the stdio
// server's mcpProtocolVersion in internal/cli; the drift test pins them
// equal so the wire never moves silently.
const ProtocolVersion = "2024-11-05"

// Capabilities is what CapabilityDiscovery serves: the wire version,
// the MCP contract version, and the tools/resources/prompts served.
type Capabilities struct {
	ProtocolVersion string     `json:"protocolVersion"`
	ContractVersion string     `json:"contractVersion"`
	Tools           []Tool     `json:"tools"`
	Resources       []Resource `json:"resources"`
	Prompts         []Prompt   `json:"prompts"`
}

// CapabilityDiscovery returns the protocol version plus the
// tools/resources/prompts served.
func CapabilityDiscovery() Capabilities {
	return Capabilities{
		ProtocolVersion: ProtocolVersion,
		ContractVersion: ContractVersion,
		Tools:           Tools(),
		Resources:       Resources(),
		Prompts:         Prompts(),
	}
}
