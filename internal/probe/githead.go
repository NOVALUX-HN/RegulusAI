package probe

import (
	"context"

	"regulus-ai/internal/gates"
)

// gitHeadBounded resolves HEAD with the S3 git bound (30s, same as
// gates.changedFiles): a wedged git must never hang a probe. The probe
// runs themselves keep their own (per-gate) timeouts — only the HEAD
// probe is bounded here.
func gitHeadBounded(r gates.CmdRunner) string {
	ctx, cancel := context.WithTimeout(context.Background(), gates.GitHeadTimeout)
	defer cancel()
	return gates.GitHead(ctx, r)
}
