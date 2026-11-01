package cli

import (
	"context"

	"regulus-ai/internal/gates"
)

// gitHeadBounded resolves HEAD with the S3 git bound (30s, same as
// gates.changedFiles): a wedged git must never hang a CLI command.
func gitHeadBounded(r gates.CmdRunner) string {
	ctx, cancel := context.WithTimeout(context.Background(), gates.GitHeadTimeout)
	defer cancel()
	return gates.GitHead(ctx, r)
}

// gitTimeout bounds one git read (rev-list, status) with the same S3
// bound as the HEAD probe.
func gitTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), gates.GitHeadTimeout)
}
