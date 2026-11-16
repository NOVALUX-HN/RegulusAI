// Package cmdx hosts the F11/F12 command handlers extracted from
// internal/cli to keep the cli composition root under its coupling
// budget (internal/size checkCoupling). cli remains the only dispatch
// table; every handler here takes the minimal IO below instead of the
// concrete cli.CLI receiver, so cli keeps one edge (cli->cmdx) instead
// of one per feature package. Behavior, flags, messages and exit codes
// are byte-identical to the former cli handlers.
package cmdx
