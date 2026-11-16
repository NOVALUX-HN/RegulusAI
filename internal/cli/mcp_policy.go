package cli

import (
	"regulus-ai/internal/cmdx"
	"regulus-ai/internal/mcp"
)

// Thin forwarders to internal/cmdx (F11.8 extraction): behavior
// byte-identical, cli no longer imports observability for these paths.
// mcp stays because the signatures name mcp types (callers in mcp.go
// are unchanged).
func (c CLI) mcpPrincipal() mcp.Principal {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MCPPrincipal()
}

func (c CLI) mcpAuthorize(name string, args map[string]any) mcp.Decision {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MCPAuthorize(name, args)
}

func (c CLI) mcpAuditDecision(p mcp.Principal, name string, args map[string]any, dec mcp.Decision, code int) {
	cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MCPAuditDecision(p, name, args, dec, code)
}
